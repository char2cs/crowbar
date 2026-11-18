//! A large heading with an optional explanation, as used above sections and pages.

use gpui::{App, ParentElement, Styled, div, px};
use ui::{Surface, Theme, UiText};

use crate::frame::spacing;

const TITLE: f32 = 32.0;
const TITLE_LINE: f32 = 48.0;
const TEXT_LINE: f32 = 18.0;

pub(crate) fn title_block(
    title: &str,
    description: Option<&str>,
    cx: &App,
) -> gpui::Div {
    let theme = *Theme::of(cx);
    let palette = theme.palette(Surface::Window);
    let note = description.map(|text| {
        div()
            .font_family(theme.ui_family())
            .text_size(UiText::Sm.size(&theme))
            .line_height(px(TEXT_LINE))
            .text_color(palette.muted_foreground())
            .child(text.to_owned())
    });
    div()
        .mb(spacing::COMFORTABLE)
        .px(spacing::TIGHT)
        .py(spacing::SNUG)
        .child(
            div()
                .font_family(theme.heading_family())
                .text_size(px(TITLE))
                .line_height(px(TITLE_LINE))
                .text_color(palette.foreground())
                .child(title.to_owned()),
        )
        .children(note)
}

#[cfg(test)]
mod tests {
    use gpui::IntoElement as _;

    use super::*;
    use crate::test_support::measure;

    #[gpui::test]
    fn a_title_alone_is_a_forty_eight_pixel_line_with_twelve_of_padding_and_eight_below(
        cx: &mut gpui::TestAppContext
    ) {
        let bounds = measure(cx, |_, cx| {
            title_block("Icons", None, cx).into_any_element()
        });
        assert_eq!(f32::from(bounds.size.height), 68.0);
    }
}
