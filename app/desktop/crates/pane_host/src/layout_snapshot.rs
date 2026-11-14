use model::{Layout, PaneInstance, PaneSlot, Rect};
use protocol::PaneId;
use webview::{Bounds, ParentWindow};

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
    /// Snapshot of `layout` for a content area `content` inside `parent`: every pane of every
    /// tab, and the panes of the active tab with their rectangles.
    #[must_use]
    pub fn from_layout(layout: &Layout, parent: ParentWindow, content: Bounds) -> Self {
        let panes = layout
            .tabs()
            .iter()
            .flat_map(|tab| tab.pane_ids().into_iter().filter_map(|id| tab.pane(&id).cloned()))
            .collect();
        let active = layout.active_tab();
        let visible = active.map(|tab| tab.layout(content_rect(content))).unwrap_or_default();
        Self {
            parent,
            panes,
            visible: visible.into_iter().map(|slot| place(slot)).collect(),
            focused: active.and_then(|tab| tab.focused().cloned()),
        }
    }
}

#[expect(
    clippy::cast_possible_truncation,
    reason = "layout rectangles are logical pixels, far below f32 precision limits"
)]
fn content_rect(content: Bounds) -> Rect {
    Rect {
        x: content.x as f32,
        y: content.y as f32,
        width: content.width as f32,
        height: content.height as f32,
    }
}

fn place(slot: PaneSlot) -> VisiblePane {
    VisiblePane {
        pane: slot.pane,
        bounds: Bounds {
            x: f64::from(slot.rect.x),
            y: f64::from(slot.rect.y),
            width: f64::from(slot.rect.width),
            height: f64::from(slot.rect.height),
        }
        ,
    }
}

#[cfg(test)]
#[path = "layout_snapshot_tests.rs"]
mod tests;
