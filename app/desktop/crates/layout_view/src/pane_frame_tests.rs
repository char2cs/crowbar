use model::Rect;

use super::*;
use crate::sidebar_edge::SidebarEdge;

const CONTENT: Rect = Rect {
    x: 296.0,
    y: 43.0,
    width: 904.0,
    height: 757.0,
};
const WITH_STRIP: ChromeContext = ChromeContext {
    sidebar: SidebarEdge::Left,
    tab_strip: true,
};

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
fn a_lone_pane_with_the_sidebar_open_has_a_top_and_left_border_and_one_round_corner() {
    let frame = PaneFrame::new(CONTENT, CONTENT, WITH_STRIP);
    assert_eq!(
        borders(&frame),
        [1.0, 1.0, 0.0, 0.0],
        "left top right bottom"
    );
    assert_eq!(corners(&frame), [10.0, 0.0, 0.0, 0.0], "tl tr bl br");
    assert_eq!(
        frame.outer,
        Rect {
            x: 296.0,
            y: 44.0,
            width: 904.0,
            height: 756.0
        },
        "top margin 1px"
    );
}

#[test]
fn a_collapsed_sidebar_removes_the_left_border_and_corner() {
    let chrome = ChromeContext {
        sidebar: SidebarEdge::Closed,
        tab_strip: true,
    };
    let frame = PaneFrame::new(CONTENT, CONTENT, chrome);
    assert_eq!(
        borders(&frame),
        [0.0, 1.0, 0.0, 0.0],
        "left top right bottom"
    );
    assert_eq!(corners(&frame), [0.0; 4], "square corners");
}

#[test]
fn the_left_pane_of_a_split_rounds_both_top_corners_and_keeps_a_right_margin() {
    let slot = Rect {
        x: 296.0,
        y: 43.0,
        width: 449.0,
        height: 757.0,
    };
    let frame = PaneFrame::new(slot, CONTENT, WITH_STRIP);
    assert_eq!(corners(&frame), [10.0, 10.0, 0.0, 0.0], "tl tr bl br");
    assert_eq!(
        borders(&frame),
        [1.0, 1.0, 1.0, 0.0],
        "left top right bottom"
    );
    assert_eq!(frame.outer.width, 448.0, "1px right margin");
}

#[test]
fn the_right_pane_of_a_split_keeps_a_left_margin_and_has_no_right_edge() {
    let slot = Rect {
        x: 751.0,
        y: 43.0,
        width: 449.0,
        height: 757.0,
    };
    let frame = PaneFrame::new(slot, CONTENT, WITH_STRIP);
    assert_eq!(
        borders(&frame),
        [1.0, 1.0, 0.0, 0.0],
        "left top right bottom"
    );
    assert_eq!(corners(&frame), [10.0, 0.0, 0.0, 0.0], "tl tr bl br");
    assert_eq!(frame.outer.x, 752.0, "1px left margin");
}

#[test]
fn a_pane_above_another_keeps_a_bottom_margin_border_and_round_bottom_corners() {
    let slot = Rect {
        x: 296.0,
        y: 43.0,
        width: 904.0,
        height: 375.0,
    };
    let frame = PaneFrame::new(slot, CONTENT, WITH_STRIP);
    assert_eq!(
        borders(&frame),
        [1.0, 1.0, 0.0, 1.0],
        "left top right bottom"
    );
    assert_eq!(corners(&frame), [10.0, 0.0, 10.0, 0.0], "tl tr bl br");
    assert_eq!(frame.outer.height, 373.0, "1px top and bottom margin");
}

#[test]
fn without_a_tab_strip_the_top_row_sits_seven_pixels_down() {
    let chrome = ChromeContext {
        sidebar: SidebarEdge::Left,
        tab_strip: false,
    };
    let frame = PaneFrame::new(CONTENT, CONTENT, chrome);
    assert_eq!(frame.outer.y, 50.0, "7px title margin");
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

#[test]
fn a_sidebar_on_the_right_mirrors_the_borders() {
    let chrome = ChromeContext {
        sidebar: SidebarEdge::Right,
        tab_strip: true,
    };
    let frame = PaneFrame::new(CONTENT, CONTENT, chrome);
    assert_eq!(
        borders(&frame),
        [0.0, 1.0, 1.0, 0.0],
        "left top right bottom"
    );
    assert_eq!(corners(&frame), [0.0, 10.0, 0.0, 0.0], "tl tr bl br");
}
