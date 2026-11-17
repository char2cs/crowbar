use std::time::Duration;

use crate::connection::serve;
use crate::ending::Ending;
use crate::peer_connector::PeerConnector;
use crate::peer_session::PeerSession;

const RECONNECT_AFTER: Duration = Duration::from_secs(2);

/// Keeps this window attached to the daemon as its browser peer until the window is gone.
///
/// The daemon speaks first on every connection and the window answers with its tabs, so a
/// reconnect needs nothing more than a new connection.
pub async fn run_peer<C: PeerConnector>(
    connector: &C,
    session: PeerSession,
) {
    while attach_once(connector, &session).await != Ending::WindowGone {
        tokio::time::sleep(RECONNECT_AFTER).await;
    }
}

async fn attach_once<C: PeerConnector>(
    connector: &C,
    session: &PeerSession,
) -> Ending {
    match connector.connect().await {
        Ok(mut stream) => serve(&mut stream, session).await,
        Err(error) => {
            tracing::debug!(%error, "the browser peer could not reach the daemon");
            Ending::Lost
        },
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use serde_json::json;
    use tokio::sync::watch;

    use super::*;
    use crate::error::Error;
    use crate::replies::Replies;
    use crate::test_support::{FakeConnector, FakeStream, RecordingShell, line, tab};

    async fn attach(
        lines: Vec<Result<FakeStream, Error>>,
        session: PeerSession,
    ) {
        run_peer(&FakeConnector::offering(lines), session).await;
    }

    #[tokio::test(start_paused = true)]
    async fn after_the_daemon_drops_the_connection_the_window_attaches_again() {
        let (first, first_daemon) = line();
        let (second, mut second_daemon) = line();
        let lines = vec![Ok(first), Err(Error::Stream("not yet".into())), Ok(second)];
        let (tabs, tabs_rx) = watch::channel(vec![tab("http://a.test/")]);
        let session = PeerSession {
            tabs: tabs_rx,
            commands: Arc::new(RecordingShell::default()),
            replies: Arc::new(Replies::default()),
        };
        let attached = tokio::spawn(attach(lines, session));
        drop(first_daemon);
        second_daemon.say(&json!({"type":"hello"}));
        let reported = second_daemon.next().await;
        assert_eq!(
            reported["type"], "tabs",
            "a new connection reports the tabs again"
        );
        drop(tabs);
        attached
            .await
            .expect("the loop ends when the window is gone");
    }
}
