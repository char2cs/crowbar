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

fn near(
    a: f32,
    b: f32,
) -> bool {
    (a - b).abs() < EDGE_TOLERANCE
}

fn top_margin(
    slot: Rect,
    content: Rect,
    chrome: ChromeContext,
) -> f32 {
    if chrome.tab_strip || !near(slot.y, content.y) {
        return MARGIN;
    }
    TITLE_MARGIN
}

impl EdgeContact {
    pub(crate) fn of(
        slot: Rect,
        content: Rect,
        chrome: ChromeContext,
    ) -> Self {
        let pick = |amount: Option<f32>| amount.unwrap_or_default();
        let at_left = near(slot.x, content.x);
        let at_right = near(slot.x + slot.width, content.x + content.width);
        let at_bottom = near(slot.y + slot.height, content.y + content.height);
        let window_left = at_left && chrome.sidebar != SidebarEdge::Left;
        let window_right = at_right && chrome.sidebar != SidebarEdge::Right;
        let margin = Insets {
            left: pick((!at_left).then_some(MARGIN)),
            top: top_margin(slot, content, chrome),
            right: pick((!at_right).then_some(MARGIN)),
            bottom: pick((!at_bottom).then_some(MARGIN)),
        };
        let border = Insets {
            left: pick((!window_left).then_some(BORDER)),
            top: BORDER,
            right: pick((!window_right).then_some(BORDER)),
            bottom: pick((!at_bottom).then_some(BORDER)),
        };
        let radius = Corners {
            top_left: pick((!window_left).then_some(RADIUS)),
            top_right: pick((!window_right).then_some(RADIUS)),
            bottom_left: pick((!(window_left || at_bottom)).then_some(RADIUS)),
            bottom_right: pick((!(window_right || at_bottom)).then_some(RADIUS)),
        };
        Self {
            margin,
            border,
            radius,
        }
    }
}

#[cfg(test)]
mod tests {
    use model::Rect;

    use super::EdgeContact;
    use crate::chrome_context::ChromeContext;

    const CONTENT: Rect = Rect {
        x: 0.0,
        y: 0.0,
        width: 400.0,
        height: 300.0,
    };

    #[test]
    fn a_pane_filling_the_content_touches_every_edge_and_has_no_side_margins() {
        let contact = EdgeContact::of(CONTENT, CONTENT, ChromeContext::default());
        assert_eq!(
            contact.margin.left, 0.0,
            "no margin at the left window edge"
        );
        assert_eq!(
            contact.margin.bottom, 0.0,
            "no margin at the bottom window edge"
        );
        assert_eq!(contact.border.left, 0.0, "no border on a window edge");
    }

    #[test]
    fn an_inner_pane_keeps_one_pixel_margins_and_borders() {
        let slot = Rect {
            x: 100.0,
            y: 50.0,
            width: 100.0,
            height: 100.0,
        };
        let contact = EdgeContact::of(slot, CONTENT, ChromeContext::default());
        assert_eq!(contact.margin.left, 1.0, "inner margin");
        assert_eq!(contact.border.right, 1.0, "inner border");
        assert_eq!(contact.radius.bottom_left, 10.0, "inner corners are round");
    }
}
