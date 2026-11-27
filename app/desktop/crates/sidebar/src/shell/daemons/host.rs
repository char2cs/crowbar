use crate::shell::daemons::host_id::HostId;
use crate::shell::daemons::host_kind::HostKind;
use crate::shell::daemons::host_state::HostState;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Host {
    pub id: HostId,
    pub name: String,
    pub kind: HostKind,
    pub state: HostState,
    pub endpoint: Option<String>,
}
