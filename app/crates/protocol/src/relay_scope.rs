use crate::extension_id::ExtensionId;
use crate::host_id::HostId;
use crate::pane_id::PaneId;
use crate::pane_kind::PaneKind;

/// Who a WebSocket relay endpoint is for.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RelayScope {
    /// The daemon host the stream connects to.
    pub host: HostId,
    /// The pane the token is issued to.
    pub pane: PaneId,
    /// Tokens are scoped by pane kind.
    pub kind: PaneKind,
    /// The extension for extension panes.
    pub extension: Option<ExtensionId>,
}
