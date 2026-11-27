use protocol::PaneId;
use webview::{Bounds, Corners};

#[derive(Debug, Clone, PartialEq)]
pub struct VisiblePane {
    pub pane: PaneId,
    pub bounds: Bounds,
    pub corners: Corners,
}
