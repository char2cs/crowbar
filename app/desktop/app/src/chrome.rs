use layout_view::{ChromeContext, SidebarEdge};
use sidebar::{DockSide, Preferences, Visibility};
use viewtabs::SidebarState;

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
mod tests {
    use super::*;

    fn preferences(
        visibility: Visibility,
        side: DockSide,
    ) -> Preferences {
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
}
