use super::*;

fn drag(side: DockSide) -> ResizeDrag {
    let mut panel = Panel::default();
    panel.dock_to(side);
    ResizeDrag::begin(294.0, 294.0, &panel, 1200.0)
}

#[test]
fn dragging_right_grows_a_left_dock() {
    let width = drag(DockSide::Left).width_at(420.0);
    assert!((width - 420.0).abs() < 0.01, "420 expected, got {width}");
}

#[test]
fn dragging_left_grows_a_right_dock() {
    let width = drag(DockSide::Right).width_at(168.0);
    assert!((width - 420.0).abs() < 0.01, "mirrored growth, got {width}");
}

#[test]
fn the_minimum_clamps_until_the_pointer_passes_the_collapse_point() {
    let drag = drag(DockSide::Left);
    assert!(
        (drag.width_at(200.0) - 250.0).abs() < 0.01,
        "clamped to the minimum"
    );
    assert!(drag.width_at(100.0).abs() < 0.01, "collapsed below 125");
    assert!(
        (drag.width_at(200.0) - 250.0).abs() < 0.01,
        "reopens when the pointer returns"
    );
}

#[test]
fn cancelling_restores_the_start_width() {
    assert!(
        (drag(DockSide::Left).cancelled_width() - 294.0).abs() < 0.01,
        "start width"
    );
}
