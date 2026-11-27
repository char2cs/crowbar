use protocol::PaneOrigin;

use crate::hosts::Target;

#[derive(Debug, Clone)]
pub(crate) struct UpstreamRequest {
    pub(crate) target: Target,
    pub(crate) origin: PaneOrigin,
    pub(crate) path_and_query: String,
    pub(crate) max_message: usize,
}
