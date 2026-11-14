//! Repository and space marks: letter tiles, emoji, images and the books glyph.

use gpui::StyledImage as _;
use gpui::{AnyElement, FontWeight, IntoElement, ParentElement, Pixels, Styled, div, img, px};
use ui::{Icon, IconName, Palette, radius};

use crate::mark::Mark;

const LETTER_SIZE: f32 = 11.0;
const EMOJI_SIZE: f32 = 14.0;

pub(crate) fn mark_view(mark: &Mark, size: Pixels, palette: &Palette) -> AnyElement {
    let frame = div()
        .flex()
        .flex_none()
        .items_center()
        .justify_center()
        .size(size);
    match mark {
        Mark::Letter { letter, tint } => frame
            .rounded(radius::SM)
            .bg(tint.color())
            .text_size(px(LETTER_SIZE))
            .font_weight(FontWeight::BOLD)
            .text_color(palette.primary_foreground())
            .child(letter.to_string())
            .into_any_element(),
        Mark::Emoji(text) => frame
            .text_size(px(EMOJI_SIZE))
            .child(text.clone())
            .into_any_element(),
        Mark::Image(path) => frame
            .child(
                img(path.clone())
                    .size(size)
                    .rounded(radius::MD)
                    .object_fit(gpui::ObjectFit::Cover),
            )
            .into_any_element(),
        Mark::Fallback => frame
            .child(
                Icon::new(IconName::Books)
                    .size(px(16.0))
                    .color(palette.foreground()),
            )
            .into_any_element(),
    }
}

#[cfg(test)]
#[path = "mark_view_tests.rs"]
mod tests;
