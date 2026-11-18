//! A titled group of examples.

use gpui::{FontWeight, IntoElement, ParentElement, Pixels, SharedString, Styled, div, px};
use ui::{Surface, Theme, spacing};

const CAPTION_SIZE: Pixels = px(11.0);

pub(crate) fn section(
    title: &'static str,
    body: impl IntoElement,
    theme: &Theme,
) -> impl IntoElement {
    let palette = theme.palette(Surface::Window);
    let caption = div()
        .text_size(CAPTION_SIZE)
        .font_weight(FontWeight::MEDIUM)
        .text_color(palette.muted_foreground())
        .child(SharedString::from(title));
    div()
        .flex()
        .flex_col()
        .gap(spacing::STEP_2)
        .child(caption)
        .child(body)
}

pub(crate) fn row() -> gpui::Div {
    div().flex().items_center().gap(spacing::STEP_2)
}
