//! A centred muted message filling an empty panel.

use gpui::{AnyElement, App, IntoElement, ParentElement, Styled, div, px};
use ui::{Surface, Theme};

pub(crate) fn panel_message(text: &'static str, cx: &App) -> AnyElement {
    let palette = Theme::of(cx).palette(Surface::Chrome);
    div()
        .flex()
        .items_center()
        .justify_center()
        .min_h(px(96.0))
        .px(px(12.0))
        .py(px(24.0))
        .text_size(px(13.0))
        .text_color(palette.muted_foreground())
        .child(text)
        .into_any_element()
}

#[cfg(test)]
#[path = "panel_message_tests.rs"]
mod tests;
