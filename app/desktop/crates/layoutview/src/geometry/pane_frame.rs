use layout::Rect;

use crate::geometry::chrome_context::ChromeContext;
use crate::geometry::corners::Corners;
use crate::geometry::insets::Insets;
use crate::geometry::sidebar_edge::SidebarEdge;

const RADIUS: f32 = 10.0;
const BORDER: f32 = 1.0;
const MARGIN: f32 = 1.0;
const TITLE_MARGIN: f32 = 7.0;
const EDGE_TOLERANCE: f32 = 0.5;

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) struct PaneFrame {
    pub(crate) outer: Rect,
    pub(crate) border: Insets,
    pub(crate) radius: Corners,
    pub(crate) hole: Rect,
}

impl PaneFrame {
    pub(crate) fn new(
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
        let outer = shrink(slot, margin);
        Self {
            outer,
            border,
            radius,
            hole: shrink(outer, border),
        }
    }
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

fn shrink(
    rect: Rect,
    by: Insets,
) -> Rect {
    Rect {
        x: rect.x + by.left,
        y: rect.y + by.top,
        width: (rect.width - by.left - by.right).max(0.0),
        height: (rect.height - by.top - by.bottom).max(0.0),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const fn rect(
        x: f32,
        width: f32,
        height: f32,
    ) -> Rect {
        Rect {
            x,
            y: 43.0,
            width,
            height,
        }
    }

    const CONTENT: Rect = rect(296.0, 904.0, 757.0);
    const LEFT_HALF: Rect = rect(296.0, 449.0, 757.0);
    const RIGHT_HALF: Rect = rect(751.0, 449.0, 757.0);
    const TOP_HALF: Rect = rect(296.0, 904.0, 375.0);

    const fn chrome(sidebar: SidebarEdge) -> ChromeContext {
        let side = match sidebar {
            SidebarEdge::Right => layout::SidebarSide::Right,
            SidebarEdge::Left | SidebarEdge::Closed => layout::SidebarSide::Left,
        };
        ChromeContext {
            sidebar,
            side,
            tab_strip: true,
        }
    }

    const LEFT: ChromeContext = chrome(SidebarEdge::Left);
    const NO_STRIP: ChromeContext = ChromeContext {
        tab_strip: false,
        ..LEFT
    };

    /// The sides with a border and the rounded corners of `frame`.
    fn edges(frame: &PaneFrame) -> (String, String) {
        let (border, radius) = (frame.border, frame.radius);
        let sides = [border.left, border.top, border.right, border.bottom];
        let corners = [
            radius.top_left,
            radius.top_right,
            radius.bottom_left,
            radius.bottom_right,
        ];
        let named = |values: [f32; 4], names: [&str; 4]| {
            let on = names.iter().zip(values).filter(|(_, value)| *value > 0.0);
            on.map(|(name, _)| *name).collect::<Vec<_>>().join(" ")
        };
        let corners = named(corners, ["tl", "tr", "bl", "br"]);
        (named(sides, ["l", "t", "r", "b"]).replace(' ', ""), corners)
    }

    type Case = (Rect, ChromeContext, [f32; 4], &'static str, &'static str);

    /// Slot and surroundings; then the bordered box, its bordered sides and its round corners.
    const CASES: &[Case] = &[
        (CONTENT, LEFT, [296.0, 44.0, 904.0, 756.0], "lt", "tl"),
        (
            CONTENT,
            chrome(SidebarEdge::Closed),
            [296.0, 44.0, 904.0, 756.0],
            "t",
            "",
        ),
        (
            CONTENT,
            chrome(SidebarEdge::Right),
            [296.0, 44.0, 904.0, 756.0],
            "tr",
            "tr",
        ),
        (CONTENT, NO_STRIP, [296.0, 50.0, 904.0, 750.0], "lt", "tl"),
        (LEFT_HALF, LEFT, [296.0, 44.0, 448.0, 756.0], "ltr", "tl tr"),
        (RIGHT_HALF, LEFT, [752.0, 44.0, 448.0, 756.0], "lt", "tl"),
        (TOP_HALF, LEFT, [296.0, 44.0, 904.0, 373.0], "ltb", "tl bl"),
    ];

    #[test]
    fn margins_borders_and_corners_follow_where_the_slot_touches_the_window() {
        for (index, (slot, chrome, outer, sides, corners)) in CASES.iter().enumerate() {
            let frame = PaneFrame::new(*slot, CONTENT, *chrome);
            let boxed = [
                frame.outer.x,
                frame.outer.y,
                frame.outer.width,
                frame.outer.height,
            ];
            let seen = (boxed, edges(&frame));
            let expected = (*outer, ((*sides).to_owned(), (*corners).to_owned()));
            assert_eq!(seen, expected, "case {index}");
        }
    }
}
