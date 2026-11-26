//! In-memory fakes shared by the unit tests of this crate.

use std::io;
use std::sync::Arc;

use async_channel::{Receiver, Sender, unbounded};
use protocol::{DaemonTransport, HostId, PaneId};
use terminal::{ClientMessage, DaemonFrame};
use tokio::io::{DuplexStream, duplex};
use tokio::runtime::Handle;
use tokio::sync::mpsc::{UnboundedReceiver, UnboundedSender, unbounded_channel};

mod fake_daemon;
mod relay_server;

pub(crate) use fake_daemon::{
    CREATED, FAILED, FakeDaemon, KILL_REFUSED, LISTED, NONE_LISTED, daemon, daemon_answering,
    daemon_creating, daemon_refusing_kills,
};
pub(crate) use relay_server::{serve_held, serve_output, serve_then_drop};

use crate::connection::Connection;
use crate::ends::Ends;
use crate::report::Report;
use crate::rest::{Binding, Rest, SessionPaths};
use crate::stream::{
    Dial,
    dial::{Dialing, Link},
};
use crate::terminal_sessions::TerminalSessions;

const BUFFER: usize = 64 * 1024;

/// A dialer whose every connection is an in-memory pipe, and the server ends of those pipes.
pub(crate) fn memory_dial() -> (Dial, UnboundedReceiver<DuplexStream>) {
    let (servers, accepted) = unbounded_channel();
    let dial: Dial = Arc::new(move |_address| connect(&servers));
    (dial, accepted)
}

fn connect(servers: &UnboundedSender<DuplexStream>) -> Dialing {
    let (client, server) = duplex(BUFFER);
    let sent = servers.send(server);
    Box::pin(async move {
        sent.map_err(|_| io::Error::from(io::ErrorKind::ConnectionRefused))?;
        Ok(Box::new(client) as Link)
    })
}

/// A dialer that cannot reach anything.
pub(crate) fn refusing_dial() -> Dial {
    Arc::new(|_address| Box::pin(async { Err(io::Error::from(io::ErrorKind::ConnectionRefused)) }))
}

/// Requests of host `local` through `daemon`.
pub(crate) fn rest(daemon: &Arc<FakeDaemon>) -> Rest {
    let transport: Arc<dyn DaemonTransport> = Arc::<FakeDaemon>::clone(daemon);
    let host = HostId::new("local").unwrap();
    Rest { transport, host }
}

/// Sessions on `daemon` whose streams go through `dial`.
pub(crate) fn sessions(
    daemon: &Arc<FakeDaemon>,
    dial: Dial,
) -> TerminalSessions {
    TerminalSessions::dialing(rest(daemon), Handle::current(), "crowbar://local", dial)
}

/// The view's side of a terminal opened in pane `t1` of chat `c1`.
pub(crate) struct View {
    pub(crate) frames: Receiver<DaemonFrame>,
    pub(crate) messages: Sender<ClientMessage>,
    pub(crate) reports: Receiver<Report>,
}

impl View {
    /// Opens a terminal bound as `binding` through `sessions`.
    pub(crate) fn open(
        sessions: &TerminalSessions,
        binding: Binding,
    ) -> Self {
        let (frames, view_frames) = unbounded();
        let (view_messages, messages) = unbounded();
        let (reports, heard) = unbounded();
        let connection = Connection {
            pane: PaneId::new("t1").unwrap(),
            ends: Ends { frames, messages },
            reports,
        };
        let base = SessionPaths::chat("c1").unwrap();
        sessions.open(base, binding, connection);
        Self {
            frames: view_frames,
            messages: view_messages,
            reports: heard,
        }
    }

    /// The text of every output frame until the stream ends.
    pub(crate) async fn all_output(&self) -> String {
        let mut text = String::new();
        while let Ok(DaemonFrame::Output(bytes)) = self.frames.recv().await {
            text.push_str(&String::from_utf8_lossy(&bytes));
        }
        text
    }
}

/// Every report until the session ends.
pub(crate) async fn all_reports(reports: &Receiver<Report>) -> Vec<Report> {
    let mut told = Vec::new();
    while let Ok(report) = reports.recv().await {
        told.push(report);
    }
    told
}
