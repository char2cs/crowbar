//! A titled group of examples.

use gpui::{FontWeight, IntoElement, ParentElement, SharedString, Styled, div, px};
use ui::{Surface, Theme};

pub(crate) fn section(
    title: &'static str,
    body: impl IntoElement,
    theme: &Theme,
) -> impl IntoElement {
    let palette = theme.palette(Surface::Window);
    let caption = div()
        .text_size(px(11.0))
        .font_weight(FontWeight::MEDIUM)
        .text_color(palette.muted_foreground())
        .child(SharedString::from(title));
    div()
        .flex()
        .flex_col()
        .gap(px(8.0))
        .child(caption)
        .child(body)
}

pub(crate) fn row() -> gpui::Div {
    div().flex().items_center().gap(px(8.0))
}
