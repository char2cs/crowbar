use gpui::Entity;
use protocol::PaneId;

use crate::tabbar::strip_state::StripState;

#[derive(Clone)]
pub(crate) struct StripSeat {
    pub(crate) pane: PaneId,
    pub(crate) index: usize,
    pub(crate) tabs: usize,
    pub(crate) strip: Entity<StripState>,
}
