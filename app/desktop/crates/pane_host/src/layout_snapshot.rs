use model::{Layout, PaneInstance};
use protocol::PaneId;
use webview::ParentWindow;

use crate::visible_pane::VisiblePane;

/// What the shell tells the host after every layout change.
#[derive(Debug, Clone, PartialEq)]
pub struct LayoutSnapshot {
    /// The window web views are embedded into.
    pub parent: ParentWindow,
    /// Every pane of the layout, in every tab. Panes missing from here are destroyed.
    pub panes: Vec<PaneInstance>,
    /// The panes with a rectangle in the active tab.
    pub visible: Vec<VisiblePane>,
    /// The focused pane of the active tab.
    pub focused: Option<PaneId>,
}

impl LayoutSnapshot {
    /// Snapshot of `layout` whose active tab shows its panes in `visible`: every pane of every
    /// tab, the focus of the active tab, and the rectangles the shell's chrome leaves open.
    #[must_use]
    pub fn from_layout(layout: &Layout, parent: ParentWindow, visible: Vec<VisiblePane>) -> Self {
        let panes = layout
            .tabs()
            .iter()
            .flat_map(|tab| {
                tab.pane_ids()
                    .into_iter()
                    .filter_map(|id| tab.pane(&id).cloned())
            })
            .collect();
        Self {
            parent,
            panes,
            visible,
            focused: layout
                .active_tab()
                .and_then(|tab| tab.focused().cloned()),
        }
    }
}

#[cfg(test)]
#[path = "layout_snapshot_tests.rs"]
mod tests;
