use layout::Rect;

use crate::geometry::chrome_context::ChromeContext;
use crate::geometry::corners::Corners;
use crate::geometry::edge_contact::EdgeContact;
use crate::geometry::insets::Insets;

/// The box a pane is drawn in: margins, 1px border and corner radii, from where the pane's slot
/// sits against the layout's bounds. The web view fills `hole`. Its edges sit on whole pixels, as
/// the web view does, so a border is never painted under the web view beside it.
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
        let outer = shrink(snapped(slot), margin);
        Self {
            outer,
            border,
            radius: contact.radius,
            hole: shrink(outer, border),
        }
    }
}

fn snapped(rect: Rect) -> Rect {
    let (left, top) = (rect.x.round(), rect.y.round());
    Rect {
        x: left,
        y: top,
        width: (rect.x + rect.width).round() - left,
        height: (rect.y + rect.height).round() - top,
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
    use layout::Rect;

    use super::*;
    use crate::geometry::sidebar_edge::SidebarEdge;

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
        side: layout::SidebarSide::Left,
        tab_strip: true,
    };
    const CLOSED: ChromeContext = ChromeContext {
        sidebar: SidebarEdge::Closed,
        side: layout::SidebarSide::Left,
        tab_strip: true,
    };
    const SIDEBAR_RIGHT: ChromeContext = ChromeContext {
        sidebar: SidebarEdge::Right,
        side: layout::SidebarSide::Right,
        tab_strip: true,
    };
    const NO_STRIP: ChromeContext = ChromeContext {
        sidebar: SidebarEdge::Left,
        side: layout::SidebarSide::Left,
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
    fn a_split_on_half_pixels_draws_each_border_on_a_whole_pixel_outside_the_web_view() {
        let left = Rect {
            width: 449.5,
            ..LEFT_HALF
        };
        let right = Rect {
            x: 751.5,
            width: 448.5,
            ..RIGHT_HALF
        };
        let (left, right) = (
            PaneFrame::new(left, CONTENT, WITH_STRIP),
            PaneFrame::new(right, CONTENT, WITH_STRIP),
        );
        let whole = |value: f32| value.fract() == 0.0;
        for frame in [left, right] {
            let edges = [
                frame.outer.x,
                frame.outer.width,
                frame.hole.x,
                frame.hole.width,
            ];
            assert!(edges.into_iter().all(whole), "{frame:?}");
        }
        let left_border = left.outer.x + left.outer.width - 1.0;
        assert_eq!(left_border, left.hole.x + left.hole.width, "its own pixel");
        assert_eq!(
            (left_border, right.outer.x),
            (744.0, 753.0),
            "develop's columns"
        );
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
