use super::*;

#[test]
fn the_pill_is_thirty_two_pixels_high_and_the_close_button_twenty_four() {
    assert_eq!(TAB_HEIGHT, 32.0, "pill height");
    assert_eq!(CLOSE, 24.0, "close button");
}
