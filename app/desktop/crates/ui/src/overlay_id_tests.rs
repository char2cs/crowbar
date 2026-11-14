use super::*;

#[test]
fn exposes_the_raw_number() {
    assert_eq!(OverlayId(42).get(), 42);
}

#[test]
fn ids_order_by_number() {
    assert!(OverlayId(1) < OverlayId(2));
}
