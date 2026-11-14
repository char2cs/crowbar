use model::Rect;

use crate::chrome_context::ChromeContext;
use crate::corners::Corners;
use crate::edge_contact::EdgeContact;
use crate::insets::Insets;

/// The box a pane is drawn in: margins, 1px border and corner radii, from where the pane's slot
/// sits against the layout's bounds. The web view fills `hole`.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct PaneFrame {
    /// The bordered box, after margins.
    pub outer: Rect,
    /// Border widths per side.
    pub border: Insets,
    /// Corner radii.
    pub radius: Corners,
    /// The area inside the border where the pane's content shows.
    pub hole: Rect,
}

impl PaneFrame {
    /// The frame of the pane in `slot`, one slot of a layout filling `content`.
    #[must_use]
    pub fn new(slot: Rect, content: Rect, chrome: ChromeContext) -> Self {
        let contact = EdgeContact::of(slot, content, chrome);
        let (margin, border) = (contact.margin, contact.border);
        let outer = shrink(slot, margin);
        Self {
            outer,
            border,
            radius: contact.radius,
            hole: shrink(outer, border),
        }
    }
}

fn shrink(rect: Rect, by: Insets) -> Rect {
    Rect {
        x: rect.x + by.left,
        y: rect.y + by.top,
        width: (rect.width - by.left - by.right).max(0.0),
        height: (rect.height - by.top - by.bottom).max(0.0),
    }
}

#[cfg(test)]
#[path = "pane_frame_tests.rs"]
mod tests;
