use std::collections::VecDeque;
use std::sync::{Arc, Mutex};

use daemonapi::{Connection, Dial, Opening};
use tokio::sync::mpsc;

/// Hands out the scripted connections one per dial, then fails.
pub(crate) struct Script(Mutex<VecDeque<Connection>>);

impl Script {
    pub(crate) fn of(connections: Vec<Connection>) -> Arc<dyn Dial> {
        Arc::new(Self(Mutex::new(connections.into())))
    }
}

pub(crate) fn refused_when_empty(next: Option<Connection>) -> daemonapi::Result<Connection> {
    next.ok_or_else(|| daemonapi::Error::Stream("no more connections".into()))
}

impl Dial for Script {
    fn open(&self) -> Opening<'_> {
        let next = self.0.lock().ok().and_then(|mut queue| queue.pop_front());
        Box::pin(std::future::ready(refused_when_empty(next)))
    }
}

/// The daemon's end of a connection: the frames it sends and the requests it hears.
pub(crate) struct Peer {
    pub(crate) frames: mpsc::Sender<String>,
    pub(crate) requests: mpsc::UnboundedReceiver<String>,
}

/// A connection and the daemon's end of it.
pub(crate) fn pair() -> (Connection, Peer) {
    let (frames, incoming) = mpsc::channel(64);
    let (outgoing, requests) = mpsc::unbounded_channel();
    (Connection { incoming, outgoing }, Peer { frames, requests })
}

const READY: &str = r#"{"t":"ready","epoch":"e"}"#;
const SYNCED: &str = r#"{"t":"synced","scope":"projects","unread":[]}"#;

impl Peer {
    /// Says the daemon is ready and returns the subscription it is asked for.
    pub(crate) async fn ready(&mut self) -> String {
        self.frames.send(READY.into()).await.unwrap();
        self.requests.recv().await.unwrap()
    }

    /// Sends `rows` of the projects scope, then its sync.
    pub(crate) async fn snapshot(
        &self,
        rows: &[String],
    ) {
        for row in rows {
            self.frames.send(row.clone()).await.unwrap();
        }
        self.frames.send(SYNCED.into()).await.unwrap();
    }
}
