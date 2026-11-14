//! A large heading with an optional explanation, as used above sections and pages.

use gpui::{App, ParentElement, Styled, div, px};
use ui::font::{HEADING_FAMILY, UI_FAMILY};
use ui::{Surface, Theme, UiText};

const TITLE: f32 = 32.0;
const TITLE_LINE: f32 = 48.0;
const TEXT_LINE: f32 = 18.0;

pub(crate) fn title_block(title: &str, description: Option<&str>, cx: &App) -> gpui::Div {
    let theme = *Theme::of(cx);
    let palette = theme.palette(Surface::Window);
    let note = description.map(|text| {
        div()
            .font_family(UI_FAMILY)
            .text_size(UiText::Sm.size(&theme))
            .line_height(px(TEXT_LINE))
            .text_color(palette.muted_foreground())
            .child(text.to_owned())
    });
    div()
        .mb(px(8.0))
        .px(px(4.0))
        .py(px(6.0))
        .child(
            div()
                .font_family(HEADING_FAMILY)
                .text_size(px(TITLE))
                .line_height(px(TITLE_LINE))
                .text_color(palette.foreground())
                .child(title.to_owned()),
        )
        .children(note)
}

#[cfg(test)]
#[path = "title_block_tests.rs"]
mod tests;
