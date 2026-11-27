use std::sync::Arc;

use tokio::sync::watch;

use crate::shell::TabCommands;
use crate::shell::TabSummary;
use crate::wait::Replies;

#[derive(Debug, Clone)]
pub struct PeerSession {
    pub tabs: watch::Receiver<Vec<TabSummary>>,
    pub commands: Arc<dyn TabCommands>,
    pub replies: Arc<Replies>,
}
