use super::*;

fn assert_close(actual: f32, expected: f32, what: &str) {
    assert!(
        (actual - expected).abs() < 0.01,
        "{what}: {actual} != {expected}"
    );
}

#[test]
fn restore_clamps_the_stored_width() {
    for (stored, expected) in [(100.0, 250.0), (294.0, 294.0), (900.0, 640.0)] {
        let panel = Panel::restore(Visibility::Open, stored, DockSide::Left);
        assert_close(panel.preferred(), expected, "restored preference");
    }
}

#[test]
fn closed_track_has_no_width_and_window_pressure_does_not_touch_the_preference() {
    let mut panel = Panel::default();
    assert_close(panel.track_width(1200.0), 294.0, "open track");
    assert_close(panel.track_width(300.0), 240.0, "squeezed to 80%");
    assert_close(panel.preferred(), 294.0, "preference survives pressure");
    panel.toggle();
    assert_close(panel.track_width(1200.0), 0.0, "closed track");
}

#[test]
fn settle_collapses_below_half_the_minimum_and_clamps_otherwise() {
    let cases = [
        (124.0, 1200.0, 0.0),
        (125.0, 1200.0, 250.0),
        (400.0, 1200.0, 400.0),
        (900.0, 1200.0, 640.0),
        (900.0, 700.0, 559.2),
    ];
    for (width, container, expected) in cases {
        assert_close(Panel::settle(width, container), expected, "settled width");
    }
}

#[test]
fn commit_zero_closes_and_keeps_the_old_preference() {
    let mut panel = Panel::default();
    panel.commit(0.0);
    assert_eq!(panel.visibility(), Visibility::Closed, "zero closes");
    assert_close(panel.preferred(), 294.0, "preference kept");
    panel.commit(420.4);
    assert_eq!(panel.visibility(), Visibility::Open, "a width reopens");
    assert_close(panel.preferred(), 420.0, "rounded preference");
}

#[test]
fn narrow_windows_use_the_drawer() {
    assert_eq!(Panel::presentation(639.0), Presentation::Overlay, "narrow");
    assert_eq!(Panel::presentation(640.0), Presentation::Docked, "wide");
}

#[test]
fn dock_side_changes_without_touching_width() {
    let mut panel = Panel::default();
    panel.dock_to(DockSide::Right);
    assert_eq!(panel.side(), DockSide::Right, "side");
    assert_close(panel.preferred(), 294.0, "width");
}
