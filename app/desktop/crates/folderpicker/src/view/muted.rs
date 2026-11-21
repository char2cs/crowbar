//! The secondary-text style the dialog uses for its hints.

use gpui::{Div, Styled, div};
use ui::Palette;

use crate::look::TEXT;

pub(crate) fn muted(palette: &Palette) -> Div {
    div().text_size(TEXT).text_color(palette.muted_foreground())
}

#[cfg(test)]
mod tests {
    use ui::{Surface, Theme};

    use super::*;

    #[gpui::test]
    fn muted_text_has_the_hint_size(cx: &mut gpui::TestAppContext) {
        cx.update(|cx| ui::init(cx).expect("ui installs"));
        let palette = cx.update(|cx| *Theme::of(cx).palette(Surface::Window));
        let mut hint = muted(&palette);
        assert_eq!(hint.style().text.font_size, Some(TEXT.into()));
    }
}
