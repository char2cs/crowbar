use protocol::PaneId;
use webview::Snapshot;

/// What a web pane looked like when an overlay hid it, so the shell can keep showing it.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct FrozenPane {
    /// The pane.
    pub pane: PaneId,
    /// Its pixels.
    pub snapshot: Snapshot,
}
