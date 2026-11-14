//! Looks of a [`crate::Tabs`] list.

use gpui::{Div, Hsla, Styled, div};

use crate::palette::Palette;
use crate::radius;
use crate::shadow;

/// Visual recipe of a tab list.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum TabsStyle {
    /// `TabsList`: 32px tabs on a muted track under a raised control-surface pill (sidebar mode switch).
    #[default]
    Pill,
    /// `SegmentedControl`: 30px segments, the indicator is the background colour (pane header switch).
    Segmented,
}

impl TabsStyle {
    pub(crate) fn tab_height(self) -> f32 {
        match self {
            Self::Pill => 32.0,
            Self::Segmented => 30.0,
        }
    }

    pub(crate) fn pill(self, palette: &Palette) -> Div {
        match self {
            Self::Pill => div()
                .rounded(radius::LG)
                .border_1()
                .border_color(palette.control_surface())
                .bg(palette.control_surface())
                .shadow(shadow::xs(0.1)),
            Self::Segmented => div()
                .rounded(radius::MD)
                .bg(palette.background())
                .shadow(shadow::sm(0.05)),
        }
    }

    pub(crate) fn active_text(self, palette: &Palette) -> Hsla {
        match self {
            Self::Pill => palette.control_foreground(),
            Self::Segmented => palette.foreground(),
        }
    }
}

#[cfg(test)]
#[path = "tabs_style_tests.rs"]
mod tests;
