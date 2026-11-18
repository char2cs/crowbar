use std::ops::ControlFlow;

use browser::AgentAnswer;
use tokio::sync::watch;
use tokio::task::{JoinError, JoinSet};

use crate::error::Error;
use crate::link::agent_request::AgentRequest;
use crate::link::connection_event::ConnectionEvent;
use crate::link::ending::Ending;
use crate::link::outbound::{response_frame, tabs_frame};
use crate::link::peer_frame::PeerFrame;
use crate::link::peer_session::PeerSession;
use crate::link::peer_stream::PeerStream;
use crate::shell::AgentCommand;
use crate::shell::AgentOp;
use crate::shell::TabInfo;

type Finished = (String, AgentAnswer);

pub(crate) struct Line<'a> {
    session: &'a PeerSession,
    tabs: watch::Receiver<Vec<TabInfo>>,
    requests: JoinSet<Finished>,
    heard: Option<Vec<TabInfo>>,
}

impl<'a> Line<'a> {
    pub(crate) fn new(session: &'a PeerSession) -> Self {
        Self {
            session,
            tabs: session.tabs.clone(),
            requests: JoinSet::new(),
            heard: None,
        }
    }

    pub(crate) async fn step<S: PeerStream>(
        &mut self,
        stream: &mut S,
    ) -> ControlFlow<Ending> {
        let event = next_event(stream, &mut self.requests, &mut self.tabs).await;
        let sent = match event {
            ConnectionEvent::Lost => return ControlFlow::Break(Ending::Lost),
            ConnectionEvent::WindowGone => return ControlFlow::Break(Ending::WindowGone),
            ConnectionEvent::Frame(text) => {
                self.on_frame(&text);
                Ok(())
            },
            ConnectionEvent::Done(id, answer) => stream.send(response_frame(&id, &answer)).await,
            ConnectionEvent::TabsChanged | ConnectionEvent::Idle => Ok(()),
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
                self.requests
                    .spawn(answer_request(request, self.session.clone()));
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
    requests: &mut JoinSet<Finished>,
    tabs: &mut watch::Receiver<Vec<TabInfo>>,
) -> ConnectionEvent {
    tokio::select! {
        frame = stream.recv() => frame.map_or(ConnectionEvent::Lost, ConnectionEvent::Frame),
        Some(done) = requests.join_next() => done_event(done),
        changed = tabs.changed() => tabs_event(changed),
    }
}

fn done_event(done: Result<Finished, JoinError>) -> ConnectionEvent {
    match done {
        Ok((id, answer)) => ConnectionEvent::Done(id, answer),
        Err(error) => {
            tracing::warn!(%error, "a browser request ended without an answer");
            ConnectionEvent::Idle
        },
    }
}

fn tabs_event(changed: Result<(), watch::error::RecvError>) -> ConnectionEvent {
    changed.map_or(ConnectionEvent::WindowGone, |()| {
        ConnectionEvent::TabsChanged
    })
}

async fn answer_request(
    request: AgentRequest,
    session: PeerSession,
) -> Finished {
    let id = request.id.clone();
    let answer = carry_out(&session, request).await;
    (id, answer)
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
    use std::time::Duration;

    use tokio::sync::watch;

    use super::*;
    use crate::test_support::{RecordingShell, line, tab};
    use crate::wait::Replies;

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

    #[tokio::test(start_paused = true)]
    async fn the_requests_of_a_connection_that_ends_stop_waiting() {
        let (_tabs, tabs_rx) = watch::channel(vec![tab("http://a.test/")]);
        let replies = Arc::new(Replies::default());
        let session = PeerSession {
            tabs: tabs_rx,
            commands: Arc::new(RecordingShell::default()),
            replies: Arc::clone(&replies),
        };
        let (mut stream, daemon) = line();
        let mut at = Line::new(&session);
        let request = serde_json::json!({
            "type":"request","id":"7","op":"snapshot","chatId":"c1","tab":"b1","args":{}
        });
        daemon.say(&request);
        let heard = at.step(&mut stream).await;
        assert_eq!(heard, ControlFlow::Continue(()));
        let quiet = tokio::time::timeout(Duration::from_millis(1), at.step(&mut stream)).await;
        assert!(quiet.is_err(), "nothing else happens");
        assert!(replies.is_waiting("7"));
        drop(at);
        tokio::task::yield_now().await;
        assert!(
            !replies.is_waiting("7"),
            "the request was cancelled with its line"
        );
    }
}
