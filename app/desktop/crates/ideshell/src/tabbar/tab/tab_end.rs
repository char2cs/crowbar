use gpui::Hsla;
use ui::{IconName, IconWeight, Palette};

use crate::tabbar::tab::tab_state::TabState;

const CLOSE_GLYPH: f32 = 12.0;
const PIN_GLYPH: f32 = 14.0;
const PIN_OPACITY: f32 = 0.6;
const CLOSE_CHORD: &str = "mod+w";

/// What the button at the end of a tab does.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum TabEnd {
    /// Closes the tab.
    Close,
    /// Unpins a pinned tab.
    Unpin,
}

impl TabEnd {
    pub(crate) fn icon(self) -> IconName {
        match self {
            Self::Close => IconName::X,
            Self::Unpin => IconName::PushPin,
        }
    }

    pub(crate) fn weight(self) -> IconWeight {
        match self {
            Self::Close => IconWeight::Regular,
            Self::Unpin => IconWeight::Fill,
        }
    }

    pub(crate) fn ink(
        self,
        state: TabState,
        palette: &Palette,
    ) -> Hsla {
        match self {
            Self::Close => state.ink(palette),
            Self::Unpin => palette.secondary(),
        }
    }

    pub(crate) fn glyph(self) -> f32 {
        match self {
            Self::Close => CLOSE_GLYPH,
            Self::Unpin => PIN_GLYPH,
        }
    }

    pub(crate) fn rest_opacity(
        self,
        state: TabState,
    ) -> f32 {
        match self {
            Self::Close => state.rest_opacity(),
            Self::Unpin => PIN_OPACITY,
        }
    }

    pub(crate) fn tip(self) -> &'static str {
        match self {
            Self::Close => "Close",
            Self::Unpin => "Unpin tab",
        }
    }

    pub(crate) fn chord(self) -> Option<chord::Chord> {
        match self {
            Self::Close => chord::Chord::parse(CLOSE_CHORD),
            Self::Unpin => None,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_pinned_tab_shows_its_pin_at_rest_and_an_ordinary_one_its_cross_only_when_active() {
        type Case = (&'static str, TabEnd, TabState, IconName, f32);
        const CASES: &[Case] = &[
            ("idle", TabEnd::Close, TabState::Idle, IconName::X, 0.0),
            ("active", TabEnd::Close, TabState::Active, IconName::X, 0.6),
            (
                "pinned idle",
                TabEnd::Unpin,
                TabState::Idle,
                IconName::PushPin,
                0.6,
            ),
            (
                "pinned active",
                TabEnd::Unpin,
                TabState::Active,
                IconName::PushPin,
                0.6,
            ),
        ];
        for (name, end, state, icon, opacity) in CASES {
            assert_eq!(end.icon(), *icon, "{name}: glyph");
            let rest = end.rest_opacity(*state);
            assert!((rest - opacity).abs() < f32::EPSILON, "{name}: {rest}");
        }
    }

    #[test]
    fn the_pin_is_solid_and_in_the_secondary_colour_the_cross_is_an_outline_in_the_tab_ink() {
        let palette = ui::Theme::default().palette(ui::Surface::Window);
        assert_eq!(TabEnd::Unpin.weight(), IconWeight::Fill, "solid pin");
        assert_eq!(TabEnd::Close.weight(), IconWeight::Regular, "outline cross");
        assert_eq!(
            TabEnd::Unpin.ink(TabState::Idle, palette),
            palette.secondary(),
            "pin"
        );
        assert_eq!(
            TabEnd::Close.ink(TabState::Active, palette),
            palette.foreground(),
            "cross"
        );
    }

    #[test]
    fn the_cross_is_titled_close_with_its_chord_and_the_pin_unpin_tab_without_one() {
        let close = TabEnd::Close
            .chord()
            .map(|chord| chord.label(chord::Primary::Control));
        assert_eq!(
            (TabEnd::Close.tip(), close.as_deref()),
            ("Close", Some("Ctrl+W"))
        );
        assert_eq!(
            (TabEnd::Unpin.tip(), TabEnd::Unpin.chord()),
            ("Unpin tab", None)
        );
    }
}
