use super::*;

#[test]
fn content_that_fits_has_no_thumb() {
    assert_eq!(Thumb::locate(300.0, 0.0, 0.0), None);
    assert_eq!(Thumb::locate(6.0, 100.0, 0.0), None);
}

#[test]
fn thumb_length_is_proportional_with_a_floor() {
    let half = Thumb::locate(300.0, 300.0, 0.0).expect("thumb");
    assert!((half.length - 146.0).abs() < 0.01, "{half:?}");
    let tiny = Thumb::locate(300.0, 30_000.0, 0.0).expect("thumb");
    assert_eq!(tiny.length, 24.0);
}

#[test]
fn thumb_travels_from_margin_to_margin() {
    let top = Thumb::locate(300.0, 300.0, 0.0).expect("thumb");
    let bottom = Thumb::locate(300.0, 300.0, 300.0).expect("thumb");
    assert_eq!(top.start, 4.0);
    assert!((bottom.start + bottom.length - 296.0).abs() < 0.01, "{bottom:?}");
}

#[test]
fn dragging_maps_thumb_travel_to_scroll_offset() {
    let travel = 300.0 - 8.0 - Thumb::locate(300.0, 300.0, 0.0).expect("thumb").length;
    let after = Thumb::scrolled_after(300.0, 300.0, 0.0, travel / 2.0);
    assert!((after - 150.0).abs() < 0.01, "{after}");
    assert_eq!(Thumb::scrolled_after(300.0, 300.0, 100.0, 1.0e6), 300.0);
    assert_eq!(Thumb::scrolled_after(300.0, 0.0, 0.0, 10.0), 0.0);
}
