mod attach_event;

use std::time::Duration;

use attach_event::AttachEvent;

use crate::link::Ending;
use crate::link::PeerConnector;
use crate::link::PeerSession;
use crate::link::serve;

const RECONNECT_AFTER: Duration = Duration::from_secs(2);

/// Keeps this window attached to the daemon as its browser peer until the window is gone.
///
/// The daemon speaks first on every connection and the window answers with its tabs, so a
/// reconnect needs nothing more than a new connection.
pub async fn run_peer<C: PeerConnector>(
    connector: &C,
    session: PeerSession,
) {
    while !window_gone(&session) && attach_once(connector, &session).await != Ending::WindowGone {
        tokio::time::sleep(RECONNECT_AFTER).await;
    }
}

fn window_gone(session: &PeerSession) -> bool {
    session.tabs.has_changed().is_err()
}

async fn attach_once<C: PeerConnector>(
    connector: &C,
    session: &PeerSession,
) -> Ending {
    match connector.connect().await {
        Ok(mut stream) => serve(&mut stream, session).await,
        Err(error) => {
            crate::journal_sink::emit(&AttachEvent::DaemonUnreachable { error: &error });
            Ending::Lost
        },
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;
    use tokio::sync::watch;

    use super::*;
    use crate::error::Error;
    use crate::test_support::{FakeConnector, FakeStream, line, session, tab};

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
        let attached = tokio::spawn(attach(lines, session(tabs_rx)));
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

    #[tokio::test(start_paused = true)]
    async fn a_window_that_goes_while_the_daemon_is_down_ends_the_loop() {
        let (tabs, tabs_rx) = watch::channel(vec![tab("http://a.test/")]);
        drop(tabs);
        let ended = tokio::time::timeout(
            Duration::from_secs(60),
            attach(Vec::new(), session(tabs_rx)),
        );
        assert!(
            ended.await.is_ok(),
            "the loop kept retrying for a gone window"
        );
    }
}
