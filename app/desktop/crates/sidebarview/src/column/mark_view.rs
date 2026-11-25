//! Repository and space marks: letter tiles, emoji, images and the books glyph.

use gpui::StyledImage as _;
use gpui::{
    AnyElement, FontWeight, Hsla, IntoElement, ParentElement, Pixels, Styled, div, img, px, rgb,
};
use sidebar::{AvatarTint, Mark};
use ui::{Icon, IconName, Palette, radius};

const SMALL_BOX: f32 = 16.0;
const LARGE_BOX: f32 = 20.0;
const SMALL_TYPE: (f32, f32, f32) = (10.0, 16.0, 14.0);
const LARGE_TYPE: (f32, f32, f32) = (11.0, 18.0, 16.0);
const HEADLINE_TYPE: (f32, f32, f32) = (13.0, 20.0, 24.0);

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

fn type_sizes(size: Pixels) -> (f32, f32, f32) {
    let side = f32::from(size);
    if side <= SMALL_BOX {
        return SMALL_TYPE;
    }
    if side <= LARGE_BOX {
        return LARGE_TYPE;
    }
    HEADLINE_TYPE
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
    let (letter_size, emoji_size, glyph_size) = type_sizes(size);
    match mark {
        Mark::Letter { letter, tint } => frame
            .rounded(radius::SM)
            .bg(avatar_color(*tint))
            .text_size(px(letter_size))
            .font_weight(FontWeight::BOLD)
            .text_color(palette.primary_foreground())
            .child(letter.to_string())
            .into_any_element(),
        Mark::Emoji(text) => frame
            .text_size(px(emoji_size))
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
                    .size(px(glyph_size))
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

    type Sizes = (f32, f32, f32);

    const TYPE_CASES: [(f32, Sizes); 3] = [
        (16.0, (10.0, 16.0, 14.0)),
        (20.0, (11.0, 18.0, 16.0)),
        (24.0, (13.0, 20.0, 24.0)),
    ];

    #[test]
    fn letters_emoji_and_the_books_glyph_grow_with_the_box_as_develops_sizes() {
        for (side, sizes) in TYPE_CASES {
            assert_eq!(type_sizes(px(side)), sizes, "{side}");
        }
    }

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
