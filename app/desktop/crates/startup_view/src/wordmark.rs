use gpui::{App, IntoElement, ParentElement, Styled, div, px};
use ui::{Surface, Theme, font};

const SIZE: f32 = 44.0;
const LINE_HEIGHT_RATIO: f32 = 1.1;

pub(crate) fn wordmark(cx: &App) -> impl IntoElement + use<> {
    let palette = Theme::of(cx).palette(Surface::Window);
    div()
        .font_family(font::HEADING_FAMILY)
        .text_size(px(SIZE))
        .line_height(px(SIZE * LINE_HEIGHT_RATIO))
        .text_color(palette.foreground())
        .child("Crowbar")
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::test_support::measure;

    #[gpui::test]
    fn the_wordmark_is_larger_than_the_headline_text(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, cx| wordmark(cx));
        assert!(f32::from(bounds.size.height) >= SIZE, "{bounds:?}");
        assert!(f32::from(bounds.size.width) > SIZE, "{bounds:?}");
    }
}
