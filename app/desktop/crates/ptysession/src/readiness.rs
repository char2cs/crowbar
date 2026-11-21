//! When a daemon serves, so a session asks nothing of a daemon that is still starting.

use std::future::Future;
use std::pin::Pin;
use std::sync::Arc;

/// A wait that ends once the daemon serves.
pub type Wait = Pin<Box<dyn Future<Output = ()> + Send>>;

/// Says when the daemon a session binds through serves; the owner of the daemon's state answers.
#[derive(Clone)]
pub struct Readiness(Arc<dyn Fn() -> Wait + Send + Sync>);

impl std::fmt::Debug for Readiness {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.write_str("Readiness")
    }
}

impl Readiness {
    /// Readiness as `wait` tells it: each call waits until the daemon serves.
    #[must_use]
    pub fn new(wait: impl Fn() -> Wait + Send + Sync + 'static) -> Self {
        Self(Arc::new(wait))
    }

    /// A daemon whose readiness is not this shell's to know: sessions bind at once.
    #[must_use]
    pub fn immediate() -> Self {
        Self::new(|| Box::pin(async {}))
    }

    pub(crate) async fn wait(&self) {
        (self.0)().await;
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use async_channel::unbounded;
    use protocol::{DaemonTransport, HostId, PaneId};
    use tokio::runtime::Handle;
    use tokio::sync::watch;

    use super::*;
    use crate::connection::Connection;
    use crate::ends::Ends;
    use crate::report::Report;
    use crate::rest::{Binding, Rest, SessionPaths};
    use crate::terminal_sessions::TerminalSessions;
    use crate::test_support::{FakeDaemon, daemon, refusing_dial};

    const SETTLE_TURNS: usize = 16;

    #[derive(Debug, Clone, Copy, PartialEq, Eq)]
    enum Phase {
        Starting,
        Serving,
    }

    fn gated(phase: &watch::Sender<Phase>) -> Readiness {
        let phase = phase.subscribe();
        Readiness::new(move || Box::pin(serving(phase.clone())))
    }

    async fn serving(mut phase: watch::Receiver<Phase>) {
        phase.wait_for(|now| *now == Phase::Serving).await.unwrap();
    }

    async fn settle() {
        for _ in 0..SETTLE_TURNS {
            tokio::task::yield_now().await;
        }
    }

    fn open_terminal(
        daemon: &Arc<FakeDaemon>,
        ready: Readiness,
    ) -> (
        async_channel::Receiver<Report>,
        async_channel::Sender<terminal::ClientMessage>,
    ) {
        let transport: Arc<dyn DaemonTransport> = Arc::<FakeDaemon>::clone(daemon);
        let rest = Rest {
            transport,
            host: HostId::new("local").unwrap(),
        };
        let sessions =
            TerminalSessions::dialing(rest, Handle::current(), "crowbar://local", refusing_dial());
        let (frames, _view_frames) = unbounded();
        let (view_messages, messages) = unbounded();
        let (reports, heard) = unbounded();
        let connection = Connection {
            pane: PaneId::new("t1").unwrap(),
            ends: Ends { frames, messages },
            reports,
        };
        let base = SessionPaths::chat("c1").unwrap();
        sessions.once(ready).open(base, Binding::Create, connection);
        (heard, view_messages)
    }

    #[tokio::test]
    async fn a_session_asks_nothing_of_a_starting_daemon_and_binds_once_it_serves() {
        let daemon = daemon();
        let (phase, _) = watch::channel(Phase::Starting);
        let (heard, _view) = open_terminal(&daemon, gated(&phase));
        settle().await;
        assert!(
            daemon.seen.lock().unwrap().is_empty(),
            "asked a starting daemon"
        );
        phase.send_replace(Phase::Serving);
        assert_eq!(heard.recv().await.ok(), Some(Report::Bound("s1".into())));
    }
}
