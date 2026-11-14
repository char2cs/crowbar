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
