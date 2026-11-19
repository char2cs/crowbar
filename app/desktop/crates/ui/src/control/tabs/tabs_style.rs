//! Looks of a [`crate::Tabs`] list.

use gpui::{Div, Hsla, ParentElement, Styled, div, px};

use crate::foundation::Palette;
use crate::foundation::tokens::opacity;
use crate::foundation::tokens::radius;
use crate::foundation::tokens::shadow;

const PILL_HEIGHT: f32 = 32.0;
const SEGMENT_HEIGHT: f32 = 30.0;
const PILL_REST_TEXT: f32 = 0.7;
const CONTROL_HIGHLIGHT: f32 = 0.16;
const SEGMENT_REST_TEXT: f32 = 0.72;
const PILL_ICON_GAP: f32 = 2.0;
const SEGMENT_ICON_GAP: f32 = 6.0;

/// Visual recipe of a tab list.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum TabsStyle {
    /// `TabsList` as the sidebar mode switch draws it: 32px tabs on the sidebar's idle wash under a
    /// raised control-surface pill.
    #[default]
    Pill,
    /// `SegmentedControl`: 30px segments, the indicator is the background colour (pane header switch).
    Segmented,
}

fn top_highlight(palette: &Palette) -> Div {
    let inset = f32::from(radius::LG) - 1.0;
    div()
        .absolute()
        .top_0()
        .left(px(inset))
        .right(px(inset))
        .h(px(1.0))
        .bg(palette.control_foreground().opacity(CONTROL_HIGHLIGHT))
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
                .relative()
                .rounded(radius::LG)
                .border_1()
                .border_color(palette.control_surface())
                .bg(palette.control_surface())
                .shadow(shadow::xs(opacity::PERCENT_10))
                .child(top_highlight(palette)),
            Self::Segmented => div()
                .rounded(radius::MD)
                .bg(palette.background())
                .shadow(shadow::sm(opacity::PERCENT_5)),
        }
    }

    pub(crate) fn track(
        self,
        palette: &Palette,
    ) -> Hsla {
        match self {
            Self::Pill => palette.sidebar_element_idle(),
            Self::Segmented => palette.muted(),
        }
    }

    pub(crate) fn rest_text(
        self,
        palette: &Palette,
    ) -> Hsla {
        match self {
            Self::Pill => palette.foreground().opacity(PILL_REST_TEXT),
            Self::Segmented => palette.muted_foreground().opacity(SEGMENT_REST_TEXT),
        }
    }

    pub(crate) fn icon_gap(self) -> f32 {
        match self {
            Self::Pill => PILL_ICON_GAP,
            Self::Segmented => SEGMENT_ICON_GAP,
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

    type Look = (f32, Hsla, Hsla, f32, Hsla);

    fn look(
        style: TabsStyle,
        palette: &Palette,
    ) -> Look {
        (
            style.tab_height(),
            style.track(palette),
            style.rest_text(palette),
            style.icon_gap(),
            style.active_text(palette),
        )
    }

    #[test]
    fn each_style_draws_its_develop_recipe() {
        let palette = Theme::default().palette(Surface::Chrome);
        let cases: [(TabsStyle, Look); 2] = [
            (
                TabsStyle::Pill,
                (
                    32.0,
                    palette.sidebar_element_idle(),
                    palette.foreground().opacity(0.7),
                    2.0,
                    palette.control_foreground(),
                ),
            ),
            (
                TabsStyle::Segmented,
                (
                    30.0,
                    palette.muted(),
                    palette.muted_foreground().opacity(0.72),
                    6.0,
                    palette.foreground(),
                ),
            ),
        ];
        for (style, expected) in cases {
            assert_eq!(look(style, palette), expected, "{style:?}");
        }
    }
}
