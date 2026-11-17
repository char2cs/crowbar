use std::ops::ControlFlow;

use browser_nav::AgentAnswer;
use tokio::sync::{mpsc, watch};

use crate::agent_command::AgentCommand;
use crate::agent_op::AgentOp;
use crate::agent_request::AgentRequest;
use crate::connection_event::ConnectionEvent;
use crate::ending::Ending;
use crate::error::Error;
use crate::outbound::{response_frame, tabs_frame};
use crate::peer_frame::PeerFrame;
use crate::peer_session::PeerSession;
use crate::peer_stream::PeerStream;
use crate::tab_info::TabInfo;

type Finished = (String, AgentAnswer);

pub(crate) struct Line<'a> {
    session: &'a PeerSession,
    tabs: watch::Receiver<Vec<TabInfo>>,
    answers: mpsc::UnboundedSender<Finished>,
    finished: mpsc::UnboundedReceiver<Finished>,
    heard: Option<Vec<TabInfo>>,
}

impl<'a> Line<'a> {
    pub(crate) fn new(session: &'a PeerSession) -> Self {
        let (answers, finished) = mpsc::unbounded_channel();
        Self {
            session,
            tabs: session.tabs.clone(),
            answers,
            finished,
            heard: None,
        }
    }

    pub(crate) async fn step<S: PeerStream>(
        &mut self,
        stream: &mut S,
    ) -> ControlFlow<Ending> {
        let event = next_event(stream, &mut self.finished, &mut self.tabs).await;
        let sent = match event {
            ConnectionEvent::Lost => return ControlFlow::Break(Ending::Lost),
            ConnectionEvent::WindowGone => return ControlFlow::Break(Ending::WindowGone),
            ConnectionEvent::Frame(text) => {
                self.on_frame(&text);
                Ok(())
            },
            ConnectionEvent::Done(id, answer) => stream.send(response_frame(&id, &answer)).await,
            ConnectionEvent::TabsChanged => Ok(()),
        };
        let reported = match sent {
            Ok(()) => self.report(stream).await,
            Err(error) => Err(error),
        };
        if let Err(error) = &reported {
            tracing::debug!(%error, "the daemon connection ended");
        }
        reported.map_or(ControlFlow::Break(Ending::Lost), ControlFlow::Continue)
    }

    fn on_frame(
        &mut self,
        text: &str,
    ) {
        match PeerFrame::decode(text) {
            Some(PeerFrame::Hello) => self.heard = None,
            Some(PeerFrame::Request(request)) => {
                let task = answer_request(request, self.session.clone(), self.answers.clone());
                tokio::spawn(task);
            },
            None => tracing::debug!("the daemon sent a frame the browser peer does not know"),
        }
    }

    async fn report<S: PeerStream>(
        &mut self,
        stream: &mut S,
    ) -> Result<(), Error> {
        let current = self.tabs.borrow_and_update().clone();
        if self.heard.as_ref() == Some(&current) {
            return Ok(());
        }
        stream.send(tabs_frame(&current)).await?;
        self.heard = Some(current);
        Ok(())
    }
}

async fn next_event<S: PeerStream>(
    stream: &mut S,
    finished: &mut mpsc::UnboundedReceiver<Finished>,
    tabs: &mut watch::Receiver<Vec<TabInfo>>,
) -> ConnectionEvent {
    tokio::select! {
        frame = stream.recv() => frame.map_or(ConnectionEvent::Lost, ConnectionEvent::Frame),
        done = finished.recv() => done.map_or(ConnectionEvent::Lost, done_event),
        changed = tabs.changed() => tabs_event(changed),
    }
}

fn done_event((id, answer): Finished) -> ConnectionEvent {
    ConnectionEvent::Done(id, answer)
}

fn tabs_event(changed: Result<(), watch::error::RecvError>) -> ConnectionEvent {
    changed.map_or(ConnectionEvent::WindowGone, |()| {
        ConnectionEvent::TabsChanged
    })
}

async fn answer_request(
    request: AgentRequest,
    session: PeerSession,
    answers: mpsc::UnboundedSender<Finished>,
) {
    let id = request.id.clone();
    let answer = carry_out(&session, request).await;
    if answers.send((id, answer)).is_err() {
        tracing::debug!("a browser request finished after its connection ended");
    }
}

async fn carry_out(
    session: &PeerSession,
    request: AgentRequest,
) -> AgentAnswer {
    let op = match AgentOp::parse(&request.op, &request.args) {
        Ok(op) => op,
        Err(error) => return AgentAnswer::Failed(error.to_string()),
    };
    let pane = Some(request.tab.as_str()).filter(|tab| !tab.is_empty());
    let wait = session.replies.register(&request.id, pane);
    let limit = op.limit();
    session.commands.run(AgentCommand {
        id: request.id,
        chat_id: request.chat_id,
        tab: request.tab,
        op,
    });
    wait.wait(limit).await
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use tokio::sync::watch;

    use super::*;
    use crate::replies::Replies;
    use crate::test_support::{RecordingShell, line, tab};

    #[tokio::test(start_paused = true)]
    async fn a_frame_the_daemon_may_not_send_changes_nothing_and_a_hung_up_daemon_ends_the_line() {
        let (_tabs, tabs_rx) = watch::channel(vec![tab("http://a.test/")]);
        let session = PeerSession {
            tabs: tabs_rx,
            commands: Arc::new(RecordingShell::default()),
            replies: Arc::new(Replies::default()),
        };
        let (mut stream, daemon) = line();
        let mut at = Line::new(&session);
        daemon.say(&serde_json::json!({"type":"nonsense"}));
        assert_eq!(at.step(&mut stream).await, ControlFlow::Continue(()));
        drop(daemon);
        assert_eq!(at.step(&mut stream).await, ControlFlow::Break(Ending::Lost));
    }
}
