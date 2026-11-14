use model::Rect;

use super::*;
use crate::chrome_context::ChromeContext;

#[test]
fn the_handle_sits_centred_on_the_top_edge_of_the_box() {
    let slot = Rect {
        x: 0.0,
        y: 0.0,
        width: 400.0,
        height: 300.0,
    };
    let frame = PaneFrame::new(slot, slot, ChromeContext::default());
    let (left, top) = handle_origin(&frame);
    let (width, _) = PaneHandle::size();
    assert_eq!(
        left,
        frame.outer.x + (frame.outer.width - width) / 2.0,
        "centred"
    );
    assert_eq!(top, frame.outer.y, "on the top edge");
}

#[test]
fn the_ring_is_two_pixels_in_light_mode_and_one_in_dark() {
    assert_eq!(RING_WIDTH_LIGHT, 2.0, "light");
    assert_eq!(RING_WIDTH_DARK, 1.0, "dark");
}
