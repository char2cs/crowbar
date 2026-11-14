use super::*;

#[test]
fn the_handle_is_sixty_nine_by_twenty_one() {
    assert_eq!(
        PaneHandle::size(),
        (69.0, 21.0),
        "three 12px buttons, two 9px gaps, padding"
    );
}

#[test]
fn the_buttons_and_gaps_add_up_to_the_width() {
    let inner = 3.0 * BUTTON + 2.0 * GAP;
    assert_eq!(
        inner + 2.0 * SIDE_PADDING,
        WIDTH,
        "inner width plus padding"
    );
    assert_eq!(
        BUTTON + TOP_PADDING + BOTTOM_PADDING,
        HEIGHT,
        "button height plus padding"
    );
}
