use super::*;
use crate::dock_side::DockSide;

fn width(key: ResizeKey, current: f32, side: DockSide) -> f32 {
    let mut panel = Panel::default();
    panel.dock_to(side);
    key.apply(KeyStep::Fine, panel, (current, 1200.0))
}

#[test]
fn arrows_step_by_sixteen_and_mirror_for_a_right_dock() {
    assert!(
        (width(ResizeKey::Right, 300.0, DockSide::Left) - 316.0).abs() < 0.01,
        "left dock grows"
    );
    assert!(
        (width(ResizeKey::Left, 300.0, DockSide::Left) - 284.0).abs() < 0.01,
        "left dock shrinks"
    );
    assert!(
        (width(ResizeKey::Left, 300.0, DockSide::Right) - 316.0).abs() < 0.01,
        "right dock grows"
    );
}

#[test]
fn shift_steps_by_sixty_four() {
    let panel = Panel::default();
    let moved = ResizeKey::Right.apply(KeyStep::Coarse, panel, (300.0, 1200.0));
    assert!((moved - 364.0).abs() < 0.01, "coarse step, got {moved}");
}

#[test]
fn clamps_at_the_minimum_and_collapses_only_below_zero() {
    assert!(
        (width(ResizeKey::Left, 250.0, DockSide::Left) - 250.0).abs() < 0.01,
        "clamped"
    );
    assert!(
        width(ResizeKey::Left, 0.0, DockSide::Left).abs() < 0.01,
        "stays collapsed"
    );
}

#[test]
fn home_collapses_end_maximises_and_enter_toggles() {
    assert!(
        width(ResizeKey::Home, 300.0, DockSide::Left).abs() < 0.01,
        "home"
    );
    assert!(
        (width(ResizeKey::End, 300.0, DockSide::Left) - 640.0).abs() < 0.01,
        "end"
    );
    assert!(
        width(ResizeKey::Enter, 300.0, DockSide::Left).abs() < 0.01,
        "enter collapses"
    );
    assert!(
        (width(ResizeKey::Enter, 0.0, DockSide::Left) - 294.0).abs() < 0.01,
        "enter reopens"
    );
}
