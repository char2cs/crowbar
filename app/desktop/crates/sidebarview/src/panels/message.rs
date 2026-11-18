//! A centred muted message filling an empty panel.

use gpui::{AnyElement, App, IntoElement, ParentElement, Pixels, Styled, div, px};
use ui::{Surface, Theme};

use crate::sidebar::{SPACE_LG, SPACE_XL, TEXT_BODY};

const MIN_HEIGHT: Pixels = px(96.0);

pub(crate) fn panel_message(
    text: &'static str,
    cx: &App,
) -> AnyElement {
    let palette = Theme::of(cx).palette(Surface::Chrome);
    div()
        .flex()
        .items_center()
        .justify_center()
        .min_h(MIN_HEIGHT)
        .px(SPACE_LG)
        .py(SPACE_XL)
        .text_size(TEXT_BODY)
        .text_color(palette.muted_foreground())
        .child(text)
        .into_any_element()
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::test_support::measure;

    #[gpui::test]
    fn the_message_fills_the_panel_width_and_keeps_a_minimum_height(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, cx| panel_message("Nothing here", cx));
        assert!(f32::from(bounds.size.height) >= 96.0);
    }
}
