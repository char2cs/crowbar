use super::*;

fn preferences(visibility: Visibility, side: DockSide) -> Preferences {
    Preferences {
        visibility,
        side,
        ..Preferences::default()
    }
}

#[test]
fn an_open_sidebar_frames_the_layout_on_its_side() {
    let (chrome, state) = of(&preferences(Visibility::Open, DockSide::Right));
    assert_eq!(
        (chrome.sidebar, state),
        (SidebarEdge::Right, SidebarState::Open),
        "right"
    );
    assert!(chrome.tab_strip, "the strip is always drawn");
}

#[test]
fn a_closed_sidebar_leaves_a_toggle_on_its_side() {
    let (chrome, state) = of(&preferences(Visibility::Closed, DockSide::Left));
    assert_eq!(
        (chrome.sidebar, state),
        (SidebarEdge::Closed, SidebarState::ClosedLeft),
        "left"
    );
}
