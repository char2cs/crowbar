//! What dragging a pane out of its strip needs.

use gpui::{AnyElement, App, SharedString};
use layout::TabId;
use protocol::PaneId;

/// What dragging a pane out of its strip needs.
pub trait PaneDetach {
    /// The chat title shown on the drag ghost of the pane.
    fn label(
        &self,
        pane: &PaneId,
    ) -> SharedString;

    /// The pane drawn as its grip's drag ghost, centred on the grip; `None` draws the labelled
    /// pill.
    fn preview(
        &self,
        pane: &PaneId,
        cx: &App,
    ) -> Option<AnyElement>;

    /// A tab id nobody uses yet and the title for the tab a detached pane opens.
    fn fresh_tab(
        &self,
        pane: &PaneId,
    ) -> (TabId, String);
}
