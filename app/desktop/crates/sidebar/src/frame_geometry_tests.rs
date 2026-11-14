use super::*;

fn panel(visibility: Visibility) -> Panel {
    Panel::restore(visibility, 294.0, DockSide::Left)
}

#[test]
fn an_open_sidebar_in_a_wide_window_is_docked() {
    let geometry = FrameGeometry::new(1200.0, 294.0, panel(Visibility::Open));
    assert!(geometry.is_docked());
    assert!(!geometry.is_drawer());
}

#[test]
fn an_open_sidebar_in_a_narrow_window_is_a_drawer() {
    let geometry = FrameGeometry::new(600.0, 294.0, panel(Visibility::Open));
    assert!(geometry.is_drawer());
    assert!(!geometry.is_docked());
}

#[test]
fn a_closed_sidebar_or_a_collapsed_width_is_neither() {
    let closed = FrameGeometry::new(1200.0, 0.0, panel(Visibility::Closed));
    assert!(!closed.is_docked() && !closed.is_drawer());
    let collapsed = FrameGeometry::new(1200.0, 0.0, panel(Visibility::Open));
    assert!(!collapsed.is_docked(), "zero width never docks");
}
