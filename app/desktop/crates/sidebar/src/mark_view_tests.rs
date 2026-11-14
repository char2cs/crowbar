use std::path::PathBuf;

use gpui::{TestAppContext, px};
use ui::{Surface, Theme};

use super::*;
use crate::avatar_tint::AvatarTint;
use crate::test_support::measure;

fn height_of(cx: &mut TestAppContext, mark: Mark) -> f32 {
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
