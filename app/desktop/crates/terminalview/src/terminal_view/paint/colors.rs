//! The theme colours the painter uses besides the cell colours.

use gpui::{App, Hsla, hsla};
use ui::{Surface, Theme};

const SELECTION_ALPHA: f32 = 0.25;
const MATCH_ALPHA: f32 = 0.35;
const CURRENT_MATCH_ALPHA: f32 = 0.8;
const THUMB_LIGHTNESS: f32 = 0.44;
const THUMB_ALPHA: f32 = 0.42;
const THUMB_HOVER_ALPHA: f32 = 0.58;

#[derive(Debug, Clone, Copy)]
pub(crate) struct Colors {
    pub(crate) pane: Hsla,
    pub(crate) foreground: Hsla,
    pub(crate) selection: Hsla,
    pub(crate) matched: Hsla,
    pub(crate) current_match: Hsla,
    pub(crate) thumb: Hsla,
    pub(crate) thumb_hover: Hsla,
    pub(crate) notice_text: Hsla,
}

impl Colors {
    pub(crate) fn of(cx: &App) -> Self {
        let tokens = Theme::of(cx).palette(Surface::Window);
        let thumb = hsla(0.0, 0.0, THUMB_LIGHTNESS, 1.0);
        Self {
            pane: tokens.pane_background(),
            foreground: tokens.foreground(),
            selection: tokens.foreground().opacity(SELECTION_ALPHA),
            matched: tokens.primary().opacity(MATCH_ALPHA),
            current_match: tokens.primary().opacity(CURRENT_MATCH_ALPHA),
            thumb: thumb.opacity(THUMB_ALPHA),
            thumb_hover: thumb.opacity(THUMB_HOVER_ALPHA),
            notice_text: tokens.foreground(),
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;

    #[gpui::test]
    fn highlights_are_translucent_versions_of_theme_colours(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap_or_else(|error| panic!("{error}")));
        let colors = cx.update(|cx| Colors::of(cx));
        assert_eq!(colors.selection.a, SELECTION_ALPHA);
        assert_eq!(colors.current_match.a, CURRENT_MATCH_ALPHA);
        assert_ne!(colors.pane, colors.foreground);
    }
}
