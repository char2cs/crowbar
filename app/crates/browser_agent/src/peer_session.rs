use std::sync::Arc;

use tokio::sync::watch;

use crate::replies::Replies;
use crate::tab_commands::TabCommands;
use crate::tab_info::TabInfo;

/// What a window shares with the loop that attaches it to the daemon.
#[derive(Debug, Clone)]
pub struct PeerSession {
    /// The window's tabs; the loop reports each change to the daemon.
    pub tabs: watch::Receiver<Vec<TabInfo>>,
    /// Where the loop hands the requests it receives.
    pub commands: Arc<dyn TabCommands>,
    /// The requests waiting for the shell.
    pub replies: Arc<Replies>,
}
