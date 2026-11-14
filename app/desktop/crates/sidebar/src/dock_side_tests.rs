use super::*;

#[test]
fn growth_follows_the_dock_edge() {
    assert!(
        (DockSide::Left.growth_sign() - 1.0).abs() < f32::EPSILON,
        "left grows rightwards"
    );
    assert!(
        (DockSide::Right.growth_sign() + 1.0).abs() < f32::EPSILON,
        "right grows leftwards"
    );
}

#[test]
fn default_dock_is_left() {
    assert_eq!(
        DockSide::default(),
        DockSide::Left,
        "sidebarPosition defaults to left"
    );
}
