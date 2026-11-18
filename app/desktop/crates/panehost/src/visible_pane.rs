use protocol::PaneId;
use webview::Bounds;

/// A pane that has a rectangle in the active tab.
#[derive(Debug, Clone, PartialEq)]
pub struct VisiblePane {
    /// The pane.
    pub pane: PaneId,
    /// Its rectangle in logical pixels, relative to the window.
    pub bounds: Bounds,
}
