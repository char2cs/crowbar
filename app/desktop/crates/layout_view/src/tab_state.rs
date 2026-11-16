use gpui::{Hsla, transparent_black};
use ui::Palette;

const CLOSE_OPACITY: f32 = 0.6;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum TabState {
    Active,
    Idle,
}

impl TabState {
    pub(crate) fn fill(
        self,
        palette: &Palette,
    ) -> Hsla {
        match self {
            Self::Active => palette.sidebar_element_hover(),
            Self::Idle => transparent_black(),
        }
    }

    pub(crate) fn ink(
        self,
        palette: &Palette,
    ) -> Hsla {
        match self {
            Self::Active => palette.foreground(),
            Self::Idle => palette.muted_foreground(),
        }
    }

    pub(crate) fn rest_opacity(self) -> f32 {
        match self {
            Self::Active => CLOSE_OPACITY,
            Self::Idle => 0.0,
        }
    }
}

#[cfg(test)]
mod tests {
    use ui::{Surface, Theme};

    use super::*;

    #[test]
    fn only_the_active_tab_is_filled_and_shows_its_close_button_at_rest() {
        let palette = Theme::default().palette(Surface::Window);
        let cases = [
            (
                TabState::Active,
                palette.sidebar_element_hover(),
                CLOSE_OPACITY,
            ),
            (TabState::Idle, transparent_black(), 0.0),
        ];
        for (state, fill, rest) in cases {
            assert_eq!(state.fill(palette), fill, "{state:?} fill");
            assert!(
                (state.rest_opacity() - rest).abs() < f32::EPSILON,
                "{state:?} rest"
            );
        }
    }

    #[test]
    fn the_active_tab_reads_in_the_foreground_and_the_others_muted() {
        let palette = Theme::default().palette(Surface::Window);
        assert_eq!(
            TabState::Active.ink(palette),
            palette.foreground(),
            "active"
        );
        assert_eq!(
            TabState::Idle.ink(palette),
            palette.muted_foreground(),
            "idle"
        );
    }
}
