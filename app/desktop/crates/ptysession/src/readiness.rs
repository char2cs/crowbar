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

    pub(crate) fn immediate() -> Self {
        Self::new(|| Box::pin(async {}))
    }

    pub(crate) async fn wait(&self) {
        (self.0)().await;
    }
}

#[cfg(test)]
mod tests {
    use tokio::sync::watch;

    use super::*;
    use crate::report::Report;
    use crate::rest::Binding;
    use crate::test_support::{View, daemon, refusing_dial, sessions};

    const SETTLE_TURNS: usize = 16;

    #[derive(Debug, Clone, Copy, PartialEq, Eq)]
    enum Phase {
        Starting,
        Serving,
    }

    async fn settle() {
        for _ in 0..SETTLE_TURNS {
            tokio::task::yield_now().await;
        }
    }

    async fn serving(mut phase: watch::Receiver<Phase>) {
        phase.wait_for(|now| *now == Phase::Serving).await.unwrap();
    }

    #[tokio::test]
    async fn a_session_asks_nothing_of_a_starting_daemon_and_binds_once_it_serves() {
        let daemon = daemon();
        let (phase_now, _) = watch::channel(Phase::Starting);
        let phase = phase_now.subscribe();
        let ready = Readiness::new(move || Box::pin(serving(phase.clone())));
        let view = View::open(
            &sessions(&daemon, refusing_dial()).once(ready),
            Binding::Create,
        );
        settle().await;
        assert!(
            daemon.seen.lock().unwrap().is_empty(),
            "asked a starting daemon"
        );
        phase_now.send_replace(Phase::Serving);
        assert_eq!(
            view.reports.recv().await.ok(),
            Some(Report::Bound("s1".into()))
        );
    }
}
