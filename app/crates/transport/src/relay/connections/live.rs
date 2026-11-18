use protocol::{HostId, PaneId};
use tokio_util::sync::CancellationToken;

pub(crate) struct Live {
    pub(crate) pane: PaneId,
    pub(crate) host: HostId,
    pub(crate) cancel: CancellationToken,
}
