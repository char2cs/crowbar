//! Small muted text.

use gpui::{App, IntoElement, ParentElement, Styled, div};
use ui::font::UI_FAMILY;
use ui::{Surface, Theme, UiText};

pub(crate) fn note_text(text: &str, cx: &App) -> impl IntoElement {
    let theme = *Theme::of(cx);
    div()
        .font_family(UI_FAMILY)
        .text_size(UiText::Sm.size(&theme))
        .text_color(theme.palette(Surface::Window).muted_foreground())
        .child(text.to_owned())
}

#[cfg(test)]
#[path = "note_text_tests.rs"]
mod tests;
