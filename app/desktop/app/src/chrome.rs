use layout_view::{ChromeContext, SidebarEdge};
use sidebar::{DockSide, Preferences, Visibility};
use tab_bar::SidebarState;

/// What surrounds the pane layout for these sidebar preferences.
pub(crate) fn of(preferences: &Preferences) -> (ChromeContext, SidebarState) {
    let (edge, state) = match (preferences.visibility, preferences.side) {
        (Visibility::Open, DockSide::Left) => (SidebarEdge::Left, SidebarState::Open),
        (Visibility::Open, DockSide::Right) => (SidebarEdge::Right, SidebarState::Open),
        (Visibility::Closed, DockSide::Left) => (SidebarEdge::Closed, SidebarState::ClosedLeft),
        (Visibility::Closed, DockSide::Right) => (SidebarEdge::Closed, SidebarState::ClosedRight),
    };
    (
        ChromeContext {
            sidebar: edge,
            tab_strip: true,
        },
        state,
    )
}

#[cfg(test)]
#[path = "chrome_tests.rs"]
mod tests;
