use layout::SidebarSide;
use layoutview::{ChromeContext, SidebarEdge};
use sidebar::{DockSide, Preferences, Visibility};
use viewtabs::SidebarState;

pub(crate) fn of(preferences: &Preferences) -> (ChromeContext, SidebarState) {
    let side = match preferences.side {
        DockSide::Left => SidebarSide::Left,
        DockSide::Right => SidebarSide::Right,
    };
    let (edge, state) = match (preferences.visibility, preferences.side) {
        (Visibility::Open, DockSide::Left) => (SidebarEdge::Left, SidebarState::Open),
        (Visibility::Open, DockSide::Right) => (SidebarEdge::Right, SidebarState::Open),
        (Visibility::Closed, DockSide::Left) => (SidebarEdge::Closed, SidebarState::ClosedLeft),
        (Visibility::Closed, DockSide::Right) => (SidebarEdge::Closed, SidebarState::ClosedRight),
    };
    (
        ChromeContext {
            sidebar: edge,
            side,
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

    /// Visibility and dock in; edge, toggle state and chat side out.
    type Case = (Visibility, DockSide, SidebarEdge, SidebarState, SidebarSide);

    const CASES: [Case; 4] = [
        (
            Visibility::Open,
            DockSide::Left,
            SidebarEdge::Left,
            SidebarState::Open,
            SidebarSide::Left,
        ),
        (
            Visibility::Open,
            DockSide::Right,
            SidebarEdge::Right,
            SidebarState::Open,
            SidebarSide::Right,
        ),
        (
            Visibility::Closed,
            DockSide::Left,
            SidebarEdge::Closed,
            SidebarState::ClosedLeft,
            SidebarSide::Left,
        ),
        (
            Visibility::Closed,
            DockSide::Right,
            SidebarEdge::Closed,
            SidebarState::ClosedRight,
            SidebarSide::Right,
        ),
    ];

    #[test]
    fn the_sidebar_frames_the_layout_and_keeps_the_chat_on_its_side_open_or_closed() {
        for (visibility, dock, edge, state, side) in CASES {
            let (chrome, shown) = of(&preferences(visibility, dock));
            assert_eq!(
                (chrome.sidebar, shown, chrome.side),
                (edge, state, side),
                "{visibility:?} {dock:?}"
            );
            assert!(
                chrome.tab_strip,
                "the strip is drawn until the shell says it holds no view"
            );
        }
    }
}
