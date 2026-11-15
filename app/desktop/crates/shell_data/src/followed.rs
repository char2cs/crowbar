//! The host a picker follows: who it is and how to reach it.

use crate::daemon_client::DaemonClient;

#[derive(Debug, Clone)]
pub(crate) struct Followed {
    pub(crate) id: String,
    pub(crate) name: String,
    pub(crate) client: DaemonClient,
}
