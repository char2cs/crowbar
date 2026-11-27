mod daemon;
mod fake_connector;
mod fake_stream;
mod recording_shell;

use std::sync::Arc;

use tokio::sync::watch;

pub(crate) use daemon::{Daemon, line};
pub(crate) use fake_connector::FakeConnector;
pub(crate) use fake_stream::FakeStream;
pub(crate) use recording_shell::RecordingShell;

use crate::link::PeerSession;
use crate::shell::TabSummary;
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
