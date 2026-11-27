use crate::strip_metrics::{ROW_GAP, TOGGLE_SIZE, TRAFFIC_LIGHT_INSET};

/// Whether the sidebar is showing, and on which side.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum SidebarState {
    /// The sidebar is open on the left: it owns the toggle and sits under the window buttons.
    #[default]
    Open,
    /// Open on the right, which leaves the strip in the window's top-left corner.
    OpenRight,
    /// Collapsed on the left: the strip shows the re-open toggle before the first tab.
    ClosedLeft,
    /// Collapsed on the right: the strip shows the mirrored toggle after the last tab.
    ClosedRight,
}

impl SidebarState {
    pub(crate) fn leading_inset(self) -> f32 {
        match self {
            Self::Open => 0.0,
            Self::OpenRight | Self::ClosedLeft | Self::ClosedRight => TRAFFIC_LIGHT_INSET,
        }
    }

    pub(crate) fn toggle_width(self) -> f32 {
        match self {
            Self::Open | Self::OpenRight => 0.0,
            Self::ClosedLeft | Self::ClosedRight => TOGGLE_SIZE + ROW_GAP,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_a_sidebar_open_on_the_left_keeps_the_window_buttons_off_the_strip() {
        let cases = [
            (SidebarState::Open, 0.0, 0.0),
            (SidebarState::OpenRight, TRAFFIC_LIGHT_INSET, 0.0),
            (SidebarState::ClosedLeft, TRAFFIC_LIGHT_INSET, 34.0),
            (SidebarState::ClosedRight, TRAFFIC_LIGHT_INSET, 34.0),
        ];
        for (state, inset, toggle) in cases {
            assert_eq!(
                (state.leading_inset(), state.toggle_width()),
                (inset, toggle)
            );
        }
    }
}
