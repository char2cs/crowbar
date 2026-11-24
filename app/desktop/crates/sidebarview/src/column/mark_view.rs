//! Repository and space marks: letter tiles, emoji, images and the books glyph.

use gpui::StyledImage as _;
use gpui::{
    AnyElement, FontWeight, Hsla, IntoElement, ParentElement, Pixels, Styled, div, img, px, rgb,
};
use sidebar::{AvatarTint, Mark};
use ui::{Icon, IconName, Palette, radius};

use crate::sidebar::ICON_LARGE;

const LETTER_SIZE: f32 = 11.0;
const EMOJI_SIZE: f32 = 14.0;

const ROSE: u32 = 0x00be_123c;
const AMBER: u32 = 0x00b4_5309;
const EMERALD: u32 = 0x0004_785f;
const CYAN: u32 = 0x000e_7490;
const INDIGO: u32 = 0x0043_38ca;
const VIOLET: u32 = 0x006d_28d9;
const SLATE: u32 = 0x0031_4158;
const PINK: u32 = 0x00be_185d;

pub(crate) fn avatar_color(tint: AvatarTint) -> Hsla {
    let hex = match tint {
        AvatarTint::Rose => ROSE,
        AvatarTint::Amber => AMBER,
        AvatarTint::Emerald => EMERALD,
        AvatarTint::Cyan => CYAN,
        AvatarTint::Indigo => INDIGO,
        AvatarTint::Violet => VIOLET,
        AvatarTint::Slate => SLATE,
        AvatarTint::Pink => PINK,
    };
    rgb(hex).into()
}

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
            .bg(avatar_color(*tint))
            .text_size(px(LETTER_SIZE))
            .font_weight(FontWeight::BOLD)
            .text_color(palette.primary_foreground())
            .child(letter.to_string())
            .into_any_element(),
        Mark::Emoji(text) => frame
            .font(ui::font::emoji_font())
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
    use sidebar::AvatarTint;

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

    const ALL: [AvatarTint; 8] = [
        AvatarTint::Rose,
        AvatarTint::Amber,
        AvatarTint::Emerald,
        AvatarTint::Cyan,
        AvatarTint::Indigo,
        AvatarTint::Violet,
        AvatarTint::Slate,
        AvatarTint::Pink,
    ];

    #[test]
    fn every_tint_is_opaque() {
        for tint in ALL {
            assert!(
                (avatar_color(tint).a - 1.0).abs() < f32::EPSILON,
                "{tint:?}"
            );
        }
    }

    #[test]
    fn every_tint_is_distinct() {
        let pairs = ALL
            .iter()
            .enumerate()
            .flat_map(|(index, tint)| ALL.iter().skip(index + 1).map(move |other| (tint, other)));
        for (tint, other) in pairs {
            assert_ne!(
                avatar_color(*tint),
                avatar_color(*other),
                "{tint:?} vs {other:?}"
            );
        }
    }
}
