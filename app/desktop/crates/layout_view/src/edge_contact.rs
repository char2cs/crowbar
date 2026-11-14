use model::Rect;

use crate::chrome_context::ChromeContext;
use crate::corners::Corners;
use crate::insets::Insets;
use crate::sidebar_edge::SidebarEdge;

const RADIUS: f32 = 10.0;
const BORDER: f32 = 1.0;
const MARGIN: f32 = 1.0;
const TITLE_MARGIN: f32 = 7.0;
const EDGE_TOLERANCE: f32 = 0.5;

#[derive(Debug, Clone, Copy)]
pub(crate) struct EdgeContact {
    pub(crate) margin: Insets,
    pub(crate) border: Insets,
    pub(crate) radius: Corners,
}

fn near(a: f32, b: f32) -> bool {
    (a - b).abs() < EDGE_TOLERANCE
}

impl EdgeContact {
    pub(crate) fn of(slot: Rect, content: Rect, chrome: ChromeContext) -> Self {
        let pick = |drawn: bool, amount: f32| if drawn { amount } else { 0.0 };
        let at_left = near(slot.x, content.x);
        let at_right = near(slot.x + slot.width, content.x + content.width);
        let at_top = !chrome.tab_strip && near(slot.y, content.y);
        let at_bottom = near(slot.y + slot.height, content.y + content.height);
        let window_left = at_left && chrome.sidebar != SidebarEdge::Left;
        let window_right = at_right && chrome.sidebar != SidebarEdge::Right;
        Self {
            margin: Insets {
                left: pick(!at_left, MARGIN),
                top: if at_top { TITLE_MARGIN } else { MARGIN },
                right: pick(!at_right, MARGIN),
                bottom: pick(!at_bottom, MARGIN),
            },
            border: Insets {
                left: pick(!window_left, BORDER),
                top: BORDER,
                right: pick(!window_right, BORDER),
                bottom: pick(!at_bottom, BORDER),
            },
            radius: Corners {
                top_left: pick(!window_left, RADIUS),
                top_right: pick(!window_right, RADIUS),
                bottom_left: pick(!(window_left || at_bottom), RADIUS),
                bottom_right: pick(!(window_right || at_bottom), RADIUS),
            },
        }
    }
}

#[cfg(test)]
#[path = "edge_contact_tests.rs"]
mod tests;
