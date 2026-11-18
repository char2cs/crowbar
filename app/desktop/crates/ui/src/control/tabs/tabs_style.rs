//! Looks of a [`crate::Tabs`] list.

use gpui::{Div, Hsla, Styled, div};

use crate::foundation::Palette;
use crate::foundation::tokens::opacity;
use crate::foundation::tokens::radius;
use crate::foundation::tokens::shadow;

const PILL_HEIGHT: f32 = 32.0;
const SEGMENT_HEIGHT: f32 = 30.0;

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
            Self::Pill => PILL_HEIGHT,
            Self::Segmented => SEGMENT_HEIGHT,
        }
    }

    pub(crate) fn pill(
        self,
        palette: &Palette,
    ) -> Div {
        match self {
            Self::Pill => div()
                .rounded(radius::LG)
                .border_1()
                .border_color(palette.control_surface())
                .bg(palette.control_surface())
                .shadow(shadow::xs(opacity::PERCENT_10)),
            Self::Segmented => div()
                .rounded(radius::MD)
                .bg(palette.background())
                .shadow(shadow::sm(opacity::PERCENT_5)),
        }
    }

    pub(crate) fn active_text(
        self,
        palette: &Palette,
    ) -> Hsla {
        match self {
            Self::Pill => palette.control_foreground(),
            Self::Segmented => palette.foreground(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::foundation::Surface;
    use crate::foundation::Theme;

    #[test]
    fn heights_follow_the_spec() {
        assert!((TabsStyle::Pill.tab_height() - 32.0).abs() < f32::EPSILON);
        assert!((TabsStyle::Segmented.tab_height() - 30.0).abs() < f32::EPSILON);
    }

    #[test]
    fn selected_text_colour_differs_per_style() {
        let palette = Theme::default().palette(Surface::Window);
        assert_eq!(
            TabsStyle::Pill.active_text(palette),
            palette.control_foreground()
        );
        assert_eq!(
            TabsStyle::Segmented.active_text(palette),
            palette.foreground()
        );
    }
}
