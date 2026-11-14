//! A daemon the shell can talk to.

use crate::host_id::HostId;
use crate::host_kind::HostKind;
use crate::host_state::HostState;

/// A paired or local host with its connection state.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Host {
    /// Identity.
    pub id: HostId,
    /// The name shown.
    pub name: String,
    /// Local or remote.
    pub kind: HostKind,
    /// Connection state.
    pub state: HostState,
    /// The endpoint shown under the name in the host picker; remote hosts only.
    pub endpoint: Option<String>,
}
