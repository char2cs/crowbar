use gpui::{Hsla, transparent_black};
use ui::Palette;

const CLOSE_OPACITY: f32 = 0.6;

/// Whether a tab is the one its bar shows.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum TabState {
    /// The shown tab.
    Active,
    /// Any other tab.
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
    fn only_the_active_tab_is_filled_in_the_foreground_and_shows_its_close_button_at_rest() {
        let palette = Theme::default().palette(Surface::Window);
        let cases = [
            (
                TabState::Active,
                (palette.sidebar_element_hover(), palette.foreground()),
                CLOSE_OPACITY,
            ),
            (
                TabState::Idle,
                (transparent_black(), palette.muted_foreground()),
                0.0,
            ),
        ];
        for (state, (fill, ink), rest) in cases {
            assert_eq!(
                (state.fill(palette), state.ink(palette)),
                (fill, ink),
                "{state:?} fill and ink"
            );
            assert!(
                (state.rest_opacity() - rest).abs() < f32::EPSILON,
                "{state:?} rest"
            );
        }
    }
}
