//! What dragging a pane out of its strip needs.

use gpui::SharedString;
use layout::TabId;
use protocol::PaneId;

/// What dragging a pane out of its strip needs.
pub trait PaneDetach {
    /// The chat title shown on the drag ghost of the pane.
    fn label(
        &self,
        pane: &PaneId,
    ) -> SharedString;

    /// A tab id nobody uses yet and the title for the tab a detached pane opens.
    fn fresh_tab(
        &self,
        pane: &PaneId,
    ) -> (TabId, String);
}
