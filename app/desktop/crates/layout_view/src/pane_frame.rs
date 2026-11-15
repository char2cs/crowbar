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
    pub fn new(
        slot: Rect,
        content: Rect,
        chrome: ChromeContext,
    ) -> Self {
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
    use model::Rect;

    use super::*;
    use crate::sidebar_edge::SidebarEdge;

    const CONTENT: Rect = Rect {
        x: 296.0,
        y: 43.0,
        width: 904.0,
        height: 757.0,
    };
    const LEFT_HALF: Rect = Rect {
        x: 296.0,
        y: 43.0,
        width: 449.0,
        height: 757.0,
    };
    const RIGHT_HALF: Rect = Rect {
        x: 751.0,
        y: 43.0,
        width: 449.0,
        height: 757.0,
    };
    const TOP_HALF: Rect = Rect {
        x: 296.0,
        y: 43.0,
        width: 904.0,
        height: 375.0,
    };
    const WITH_STRIP: ChromeContext = ChromeContext {
        sidebar: SidebarEdge::Left,
        tab_strip: true,
    };
    const CLOSED: ChromeContext = ChromeContext {
        sidebar: SidebarEdge::Closed,
        tab_strip: true,
    };
    const SIDEBAR_RIGHT: ChromeContext = ChromeContext {
        sidebar: SidebarEdge::Right,
        tab_strip: true,
    };
    const NO_STRIP: ChromeContext = ChromeContext {
        sidebar: SidebarEdge::Left,
        tab_strip: false,
    };

    struct Case {
        name: &'static str,
        slot: Rect,
        chrome: ChromeContext,
        borders: [f32; 4],
        corners: [f32; 4],
    }

    const CASES: &[Case] = &[
        Case {
            name: "a lone pane with the sidebar open has a top and left border and one round corner",
            slot: CONTENT,
            chrome: WITH_STRIP,
            borders: [1.0, 1.0, 0.0, 0.0],
            corners: [10.0, 0.0, 0.0, 0.0],
        },
        Case {
            name: "a collapsed sidebar removes the left border and corner",
            slot: CONTENT,
            chrome: CLOSED,
            borders: [0.0, 1.0, 0.0, 0.0],
            corners: [0.0; 4],
        },
        Case {
            name: "the left pane of a split rounds both top corners and keeps a right border",
            slot: LEFT_HALF,
            chrome: WITH_STRIP,
            borders: [1.0, 1.0, 1.0, 0.0],
            corners: [10.0, 10.0, 0.0, 0.0],
        },
        Case {
            name: "the right pane of a split has no right edge",
            slot: RIGHT_HALF,
            chrome: WITH_STRIP,
            borders: [1.0, 1.0, 0.0, 0.0],
            corners: [10.0, 0.0, 0.0, 0.0],
        },
        Case {
            name: "a pane above another keeps a bottom border and round bottom corners",
            slot: TOP_HALF,
            chrome: WITH_STRIP,
            borders: [1.0, 1.0, 0.0, 1.0],
            corners: [10.0, 0.0, 10.0, 0.0],
        },
        Case {
            name: "a sidebar on the right mirrors the borders",
            slot: CONTENT,
            chrome: SIDEBAR_RIGHT,
            borders: [0.0, 1.0, 1.0, 0.0],
            corners: [0.0, 10.0, 0.0, 0.0],
        },
    ];

    struct Margin {
        name: &'static str,
        slot: Rect,
        chrome: ChromeContext,
        measure: fn(&PaneFrame) -> f32,
        expected: f32,
    }

    const MARGINS: &[Margin] = &[
        Margin {
            name: "1px top margin under the tab strip",
            slot: CONTENT,
            chrome: WITH_STRIP,
            measure: |frame| frame.outer.y,
            expected: 44.0,
        },
        Margin {
            name: "1px right margin of the left pane",
            slot: LEFT_HALF,
            chrome: WITH_STRIP,
            measure: |frame| frame.outer.width,
            expected: 448.0,
        },
        Margin {
            name: "1px left margin of the right pane",
            slot: RIGHT_HALF,
            chrome: WITH_STRIP,
            measure: |frame| frame.outer.x,
            expected: 752.0,
        },
        Margin {
            name: "1px top and bottom margin of the upper pane",
            slot: TOP_HALF,
            chrome: WITH_STRIP,
            measure: |frame| frame.outer.height,
            expected: 373.0,
        },
        Margin {
            name: "without a tab strip the top row sits seven pixels down",
            slot: CONTENT,
            chrome: NO_STRIP,
            measure: |frame| frame.outer.y,
            expected: 50.0,
        },
    ];

    fn corners(frame: &PaneFrame) -> [f32; 4] {
        let radius = frame.radius;
        [
            radius.top_left,
            radius.top_right,
            radius.bottom_left,
            radius.bottom_right,
        ]
    }

    fn borders(frame: &PaneFrame) -> [f32; 4] {
        let border = frame.border;
        [border.left, border.top, border.right, border.bottom]
    }

    #[test]
    fn borders_and_corners_follow_where_the_slot_touches_the_window() {
        for case in CASES {
            let frame = PaneFrame::new(case.slot, CONTENT, case.chrome);
            assert_eq!(borders(&frame), case.borders, "{}: borders", case.name);
            assert_eq!(corners(&frame), case.corners, "{}: corners", case.name);
        }
    }

    #[test]
    fn margins_shrink_the_slot_to_the_bordered_box() {
        for case in MARGINS {
            let frame = PaneFrame::new(case.slot, CONTENT, case.chrome);
            assert_eq!((case.measure)(&frame), case.expected, "{}", case.name);
        }
    }

    #[test]
    fn the_hole_is_the_box_inside_the_border() {
        let frame = PaneFrame::new(CONTENT, CONTENT, WITH_STRIP);
        assert_eq!(
            frame.hole,
            Rect {
                x: 297.0,
                y: 45.0,
                width: 903.0,
                height: 755.0
            },
            "inside the border"
        );
    }
}
