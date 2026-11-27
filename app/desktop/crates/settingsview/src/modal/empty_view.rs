use gpui::{App, IntoElement, ParentElement, Styled, div};
use ui::{ScaledText, Surface, Theme};

use crate::frame::spacing;

pub(crate) fn empty_view(cx: &App) -> impl IntoElement {
    let theme = *Theme::of(cx);
    div()
        .p(spacing::COMFORTABLE)
        .flex()
        .justify_center()
        .font_family(theme.ui_family())
        .text_size(ScaledText::Sm.size(&theme))
        .text_color(theme.palette(Surface::Window).muted_foreground())
        .child("No matching settings")
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::measure;

    #[gpui::test]
    fn the_empty_text_is_padded_and_visible(cx: &mut gpui::TestAppContext) {
        let bounds = measure(cx, |_, cx| empty_view(cx).into_any_element());
        assert!(f32::from(bounds.size.height) > 16.0);
    }
}
