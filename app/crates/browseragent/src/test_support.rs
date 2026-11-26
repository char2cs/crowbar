use std::collections::VecDeque;
use std::future::Future;
use std::sync::{Arc, Mutex};

use serde_json::Value;
use tokio::sync::{Notify, mpsc, watch};

use crate::error::Error;
use crate::link::{PeerConnector, PeerSession, PeerStream};
use crate::shell::{AgentCommand, TabCommands, TabSummary};
use crate::wait::Replies;

pub(crate) fn tab(url: &str) -> TabSummary {
    TabSummary {
        id: "b1".into(),
        chat_id: "c1".into(),
        url: url.into(),
        title: String::new(),
        loading: false,
    }
}

pub(crate) fn session(tabs: watch::Receiver<Vec<TabSummary>>) -> PeerSession {
    PeerSession {
        tabs,
        commands: Arc::new(RecordingShell::default()),
        replies: Arc::new(Replies::default()),
    }
}

pub(crate) struct Daemon {
    to_window: mpsc::UnboundedSender<String>,
    from_window: mpsc::UnboundedReceiver<String>,
}

impl Daemon {
    pub(crate) async fn next(&mut self) -> Value {
        let text = self
            .from_window
            .recv()
            .await
            .expect("the window sent a frame");
        serde_json::from_str(&text).expect("the window sent json")
    }

    pub(crate) fn say(
        &self,
        frame: &Value,
    ) {
        self.to_window
            .send(frame.to_string())
            .expect("the window listens");
    }
}

pub(crate) fn line() -> (FakeStream, Daemon) {
    let (to_window, inbound) = mpsc::unbounded_channel();
    let (outbound, from_window) = mpsc::unbounded_channel();
    let stream = FakeStream { inbound, outbound };
    let daemon = Daemon {
        to_window,
        from_window,
    };
    (stream, daemon)
}

pub(crate) struct FakeStream {
    inbound: mpsc::UnboundedReceiver<String>,
    outbound: mpsc::UnboundedSender<String>,
}

impl PeerStream for FakeStream {
    async fn recv(&mut self) -> Option<String> {
        self.inbound.recv().await
    }

    fn send(
        &mut self,
        text: String,
    ) -> impl Future<Output = Result<(), Error>> + Send {
        let sent = self
            .outbound
            .send(text)
            .map_err(|error| Error::Stream(Box::new(error)));
        std::future::ready(sent)
    }
}

pub(crate) struct FakeConnector {
    lines: Mutex<VecDeque<Result<FakeStream, Error>>>,
}

impl FakeConnector {
    pub(crate) fn offering(lines: Vec<Result<FakeStream, Error>>) -> Self {
        Self {
            lines: Mutex::new(lines.into()),
        }
    }
}

impl PeerConnector for FakeConnector {
    type Stream = FakeStream;

    fn connect(&self) -> impl Future<Output = Result<FakeStream, Error>> + Send {
        let next = self
            .lines
            .lock()
            .expect("the line list")
            .pop_front()
            .unwrap_or_else(|| Err(Error::Stream("no daemon".into())));
        std::future::ready(next)
    }
}

#[derive(Debug, Default)]
pub(crate) struct RecordingShell {
    received: Mutex<Vec<AgentCommand>>,
    arrived: Notify,
}

impl RecordingShell {
    pub(crate) async fn first(&self) -> AgentCommand {
        self.arrived.notified().await;
        self.received.lock().expect("the command list").remove(0)
    }
}

impl TabCommands for RecordingShell {
    fn run(
        &self,
        command: AgentCommand,
    ) {
        self.received
            .lock()
            .expect("the command list")
            .push(command);
        self.arrived.notify_one();
    }
}
