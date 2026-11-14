//! The text shown when a search matches nothing.

use gpui::{App, IntoElement, ParentElement, Styled, div, px};
use ui::font::UI_FAMILY;
use ui::{Surface, Theme, UiText};

pub(crate) fn empty_view(cx: &App) -> impl IntoElement {
    let theme = *Theme::of(cx);
    div()
        .p(px(8.0))
        .flex()
        .justify_center()
        .font_family(UI_FAMILY)
        .text_size(UiText::Sm.size(&theme))
        .text_color(theme.palette(Surface::Window).muted_foreground())
        .child("No matching settings")
}

#[cfg(test)]
#[path = "empty_view_tests.rs"]
mod tests;
