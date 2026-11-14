use protocol::PaneOrigin;

use crate::host_target::HostTarget;

#[derive(Debug, Clone)]
pub(crate) struct UpstreamRequest {
    pub(crate) target: HostTarget,
    pub(crate) origin: PaneOrigin,
    pub(crate) path_and_query: String,
    pub(crate) max_message: usize,
}
