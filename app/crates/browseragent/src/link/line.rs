mod line_event;

use std::ops::ControlFlow;

use browser::AgentAnswer;
use tokio::sync::watch;
use tokio::task::{JoinError, JoinSet};

use crate::error::Error;
use crate::journal_sink::emit;
use crate::link::agent_request::AgentRequest;
use crate::link::connection_event::ConnectionEvent;
use crate::link::ending::Ending;
use crate::link::outbound::{response_frame, tabs_frame};
use crate::link::peer_frame::PeerFrame;
use crate::link::peer_session::PeerSession;
use crate::link::peer_stream::PeerStream;
use crate::shell::AgentCommand;
use crate::shell::AgentOp;
use crate::shell::TabSummary;
use line_event::LineEvent;

type Finished = (String, AgentAnswer);

pub(crate) async fn serve<S: PeerStream>(
    stream: &mut S,
    session: &PeerSession,
) -> Ending {
    let mut line = Line::new(session);
    let mut flow = ControlFlow::Continue(());
    while flow.is_continue() {
        flow = line.step(stream).await;
    }
    flow.break_value().unwrap_or(Ending::Lost)
}

pub(crate) struct Line<'a> {
    session: &'a PeerSession,
    tabs: watch::Receiver<Vec<TabSummary>>,
    requests: JoinSet<Finished>,
    heard: Option<Vec<TabSummary>>,
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
            emit(&LineEvent::ConnectionEnded { error });
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
            None => emit(&LineEvent::UnknownFrame),
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
    tabs: &mut watch::Receiver<Vec<TabSummary>>,
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
            emit(&LineEvent::RequestUnanswered { error: &error });
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

    use serde_json::{Value, json};

    use super::*;
    use crate::shell::TabCommands;
    use crate::test_support::{Daemon, RecordingShell, line, session, tab};
    use crate::wait::{Delivery, Replies};

    struct Rig {
        daemon: Daemon,
        tabs: watch::Sender<Vec<TabSummary>>,
        shell: Arc<RecordingShell>,
        replies: Arc<Replies>,
    }

    fn rig() -> Rig {
        let (mut stream, daemon) = line();
        let (tabs, tabs_rx) = watch::channel(vec![tab("http://a.test/")]);
        let shell = Arc::new(RecordingShell::default());
        let replies = Arc::new(Replies::default());
        let session = PeerSession {
            tabs: tabs_rx,
            commands: Arc::clone(&shell) as Arc<dyn TabCommands>,
            replies: Arc::clone(&replies),
        };
        tokio::spawn(async move {
            serve(&mut stream, &session).await;
        });
        Rig {
            daemon,
            tabs,
            shell,
            replies,
        }
    }

    fn request(
        id: &str,
        op: &str,
        args: &Value,
    ) -> Value {
        json!({"type":"request","id":id,"op":op,"chatId":"c1","tab":"b1","args":args})
    }

    #[tokio::test(start_paused = true)]
    async fn a_hello_is_answered_with_the_tabs_and_a_change_is_reported_once() {
        let mut rig = rig();
        rig.daemon.say(&json!({"type":"hello"}));
        let first = rig.daemon.next().await;
        assert_eq!(first["tabs"][0]["url"], "http://a.test/");
        rig.tabs.send_replace(vec![tab("http://a.test/")]);
        rig.tabs.send_replace(vec![tab("http://b.test/")]);
        let second = rig.daemon.next().await;
        assert_eq!(
            second["tabs"][0]["url"], "http://b.test/",
            "an equal list is not repeated"
        );
        let more = tokio::time::timeout(Duration::from_millis(1), rig.daemon.next()).await;
        assert!(more.is_err(), "nothing more was reported");
    }

    #[tokio::test(start_paused = true)]
    async fn a_request_reaches_the_shell_and_the_shells_answer_goes_back_under_its_id() {
        let mut rig = rig();
        rig.daemon.say(&request("7", "click", &json!({"ref":"e1"})));
        let _tabs = rig.daemon.next().await;
        let received = rig.shell.first().await;
        let named = (
            received.id.as_str(),
            received.chat_id.as_str(),
            received.tab.as_str(),
        );
        assert_eq!(named, ("7", "c1", "b1"));
        assert!(matches!(received.op, AgentOp::Page { .. }));
        let delivered = rig
            .replies
            .from_page("b1", "7", AgentAnswer::Done(json!("clicked")));
        assert_eq!(delivered, Delivery::Delivered);
        let answer = rig.daemon.next().await;
        assert_eq!(
            answer,
            json!({"type":"response","id":"7","ok":true,"result":"clicked"})
        );
    }

    const CASES: &[(&str, &str, &str)] = &[
        ("navigate", r#"{"url":"file:///etc/passwd"}"#, "file"),
        ("snapshot", "{}", "no answer"),
    ];

    async fn fails_under_its_id(&(op, args, mentions): &(&str, &str, &str)) {
        let mut rig = rig();
        let args: Value = serde_json::from_str(args).unwrap();
        rig.daemon.say(&request("9", op, &args));
        let _tabs = rig.daemon.next().await;
        let answer = rig.daemon.next().await;
        assert_eq!((&answer["ok"], &answer["id"]), (&json!(false), &json!("9")));
        let error = answer["error"].as_str().unwrap();
        assert!(error.contains(mentions), "{op}: {answer}");
    }

    async fn every_failure() {
        for case in CASES {
            fails_under_its_id(case).await;
        }
    }

    #[tokio::test(start_paused = true)]
    async fn a_request_that_cannot_be_carried_out_or_is_never_answered_fails_under_its_id() {
        every_failure().await;
    }

    #[tokio::test(start_paused = true)]
    async fn a_connection_ends_as_lost_when_the_daemon_hangs_up_and_as_gone_when_the_window_does() {
        let (tabs, tabs_rx) = watch::channel(vec![tab("http://a.test/")]);
        let session = session(tabs_rx);
        let (mut stream, daemon) = line();
        let mut at = Line::new(&session);
        daemon.say(&json!({"type":"nonsense"}));
        let unknown = at.step(&mut stream).await;
        assert_eq!(
            unknown,
            ControlFlow::Continue(()),
            "an unknown frame changes nothing"
        );
        drop(daemon);
        assert_eq!(serve(&mut stream, &session).await, Ending::Lost);
        let (mut stream, _daemon) = line();
        drop(tabs);
        assert_eq!(serve(&mut stream, &session).await, Ending::WindowGone);
    }

    #[tokio::test(start_paused = true)]
    async fn the_requests_of_a_connection_that_ends_stop_waiting() {
        let (_tabs, tabs_rx) = watch::channel(vec![tab("http://a.test/")]);
        let session = session(tabs_rx);
        let replies = Arc::clone(&session.replies);
        let (mut stream, daemon) = line();
        let mut at = Line::new(&session);
        daemon.say(&request("7", "snapshot", &json!({})));
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
