mod daemon;
mod fake_connector;
mod fake_stream;
mod recording_shell;

use tokio::sync::mpsc;

use crate::tab_info::TabInfo;

pub(crate) use daemon::Daemon;
pub(crate) use fake_connector::FakeConnector;
pub(crate) use fake_stream::FakeStream;
pub(crate) use recording_shell::RecordingShell;

pub(crate) fn line() -> (FakeStream, Daemon) {
    let (to_window, inbound) = mpsc::unbounded_channel();
    let (outbound, from_window) = mpsc::unbounded_channel();
    (
        FakeStream::new(inbound, outbound),
        Daemon::new(to_window, from_window),
    )
}

pub(crate) fn tab(url: &str) -> TabInfo {
    TabInfo {
        id: "b1".into(),
        chat_id: "c1".into(),
        url: url.into(),
        title: String::new(),
        loading: false,
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;

    #[tokio::test]
    async fn what_the_daemon_says_reaches_the_window_and_what_the_window_says_reaches_the_daemon() {
        let (mut stream, mut daemon) = line();
        daemon.say(&json!({"type":"hello"}));
        assert_eq!(
            crate::peer_stream::PeerStream::recv(&mut stream)
                .await
                .as_deref(),
            Some(r#"{"type":"hello"}"#)
        );
        crate::peer_stream::PeerStream::send(&mut stream, r#"{"a":1}"#.into())
            .await
            .unwrap();
        assert_eq!(daemon.next().await, json!({"a":1}));
        assert!(daemon.nothing_waits());
        assert_eq!(tab("http://a.test/").url, "http://a.test/");
    }
}
