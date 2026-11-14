use model::{Rect, Tab};

use crate::chrome_context::ChromeContext;
use crate::pane_frame::PaneFrame;
use crate::pane_hole::PaneHole;

/// The content hole of every pane of `tab` when the layout fills `content`. The shell gives
/// these rectangles to the pane host in place of the model's raw slots, so web views sit inside
/// the pane borders the view draws.
#[must_use]
pub fn pane_holes(tab: &Tab, content: Rect, chrome: ChromeContext) -> Vec<PaneHole> {
    tab.layout(content)
        .into_iter()
        .map(|slot| PaneHole {
            rect: PaneFrame::new(slot.rect, content, chrome).hole,
            pane: slot.pane,
        })
        .collect()
}

#[cfg(test)]
#[path = "pane_holes_tests.rs"]
mod tests;
