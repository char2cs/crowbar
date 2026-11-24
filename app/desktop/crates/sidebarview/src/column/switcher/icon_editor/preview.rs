//! The large mark at the top of an icon popover.

use std::path::PathBuf;

use gpui::StyledImage as _;
use gpui::{Div, InteractiveElement as _, ParentElement, Styled, div, img, px};
use sidebar::{Mark, SpaceIcon};
use ui::{Palette, radius};

use super::IconEditor;
use crate::column::mark_view;

const PREVIEW: f32 = 56.0;
const EMOJI_PREVIEW: f32 = 24.0;
const GLYPH_PREVIEW: f32 = 24.0;

impl IconEditor {
    pub(crate) fn show(
        &mut self,
        (current, fallback): (SpaceIcon, Mark),
    ) {
        self.current = current;
        self.fallback = fallback;
    }
}

impl IconEditor {
    pub(super) fn preview(
        &self,
        palette: &Palette,
    ) -> Div {
        preview(&self.current, &self.fallback, palette)
    }
}

fn preview(
    icon: &SpaceIcon,
    fallback: &Mark,
    palette: &Palette,
) -> Div {
    let frame = div()
        .flex()
        .flex_none()
        .items_center()
        .justify_center()
        .size(px(PREVIEW))
        .rounded(radius::XL)
        .overflow_hidden()
        .bg(palette.background())
        .debug_selector(|| "icon-preview".into());
    let framed = match icon {
        SpaceIcon::Emoji(text) => frame
            .font(ui::font::emoji_font())
            .text_size(px(EMOJI_PREVIEW))
            .child(text.clone()),
        SpaceIcon::Image(path) => frame.child(
            img(PathBuf::from(path))
                .size(px(PREVIEW))
                .rounded(radius::XL)
                .object_fit(gpui::ObjectFit::Cover),
        ),
        SpaceIcon::Default => frame.child(mark_view(fallback, px(GLYPH_PREVIEW), palette)),
    };
    div().flex().justify_center().child(framed)
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use sidebar::AvatarTint;
    use ui::{Surface, Theme};

    use super::*;
    use crate::test_support::measure;

    fn height_of(
        cx: &mut TestAppContext,
        (icon, fallback): (SpaceIcon, Mark),
    ) -> f32 {
        let bounds = measure(cx, move |_, cx| {
            preview(&icon, &fallback, Theme::of(cx).palette(Surface::Window))
        });
        f32::from(bounds.size.height)
    }

    #[gpui::test]
    fn every_icon_fills_the_preview_and_a_repo_falls_back_to_its_letter(cx: &mut TestAppContext) {
        let letter = Mark::Letter {
            letter: 'C',
            tint: AvatarTint::Rose,
        };
        let cases = [
            (SpaceIcon::Default, Mark::Fallback),
            (SpaceIcon::Default, letter),
            (SpaceIcon::Emoji("⭐".into()), Mark::Fallback),
        ];
        for case in cases {
            assert_eq!(height_of(cx, case), PREVIEW);
        }
    }
}
