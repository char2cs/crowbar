use gpui::{AnyElement, App, SharedString};
use layout::TabId;
use protocol::PaneId;

pub trait PaneDetach {
    fn label(
        &self,
        pane: &PaneId,
    ) -> SharedString;

    fn preview(
        &self,
        pane: &PaneId,
        cx: &App,
    ) -> Option<AnyElement>;

    fn fresh_tab(
        &self,
        pane: &PaneId,
    ) -> (TabId, String);
}
