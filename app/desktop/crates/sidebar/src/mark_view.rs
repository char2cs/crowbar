//! Repository and space marks: letter tiles, emoji, images and the books glyph.

use gpui::StyledImage as _;
use gpui::{AnyElement, FontWeight, IntoElement, ParentElement, Pixels, Styled, div, img, px};
use ui::{Icon, IconName, Palette, radius};

use crate::mark::Mark;
use crate::scale::ICON_LARGE;

const LETTER_SIZE: f32 = 11.0;
const EMOJI_SIZE: f32 = 14.0;

pub(crate) fn mark_view(
    mark: &Mark,
    size: Pixels,
    palette: &Palette,
) -> AnyElement {
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
                    .size(ICON_LARGE)
                    .color(palette.foreground()),
            )
            .into_any_element(),
    }
}

#[cfg(test)]
mod tests {
    use std::path::PathBuf;

    use gpui::{TestAppContext, px};
    use ui::{Surface, Theme};

    use super::*;
    use crate::avatar_tint::AvatarTint;
    use crate::test_support::measure;

    fn height_of(
        cx: &mut TestAppContext,
        mark: Mark,
    ) -> f32 {
        let bounds = measure(cx, move |_, cx| {
            let palette = Theme::of(cx).palette(Surface::Chrome);
            mark_view(&mark, px(20.0), palette)
        });
        f32::from(bounds.size.height)
    }

    #[gpui::test]
    fn every_kind_of_mark_fills_its_square(cx: &mut TestAppContext) {
        let marks = [
            Mark::Letter {
                letter: 'D',
                tint: AvatarTint::Slate,
            },
            Mark::Emoji("x".into()),
            Mark::Image(PathBuf::from("/nonexistent.png")),
            Mark::Fallback,
        ];
        for mark in marks {
            assert_eq!(height_of(cx, mark.clone()), 20.0, "{mark:?}");
        }
    }
}
