use gpui::{AnyElement, App, SharedString, Window};
use model::TabId;
use protocol::PaneId;

/// What the layout view asks of the shell about a pane; the shell implements it, the view never
/// looks a pane up itself.
pub trait PaneSource {
    /// What fills the pane's content hole. The real shell returns an empty element, because the
    /// pane host's web view covers the hole.
    fn content(&self, pane: &PaneId, window: &mut Window, cx: &mut App) -> AnyElement;

    /// The chat title shown on the drag ghost of the pane.
    fn label(&self, pane: &PaneId) -> SharedString;

    /// A tab id nobody uses yet and the title for the tab a detached pane opens.
    fn fresh_tab(&self, pane: &PaneId) -> (TabId, String);
}

#[cfg(test)]
#[path = "pane_source_tests.rs"]
mod tests;
