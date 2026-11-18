//! Small muted text.

use gpui::{App, IntoElement, ParentElement, Styled, div};
use ui::{ScaledText, Surface, Theme};

pub(crate) fn note_text(
    text: &str,
    cx: &App,
) -> impl IntoElement {
    let theme = *Theme::of(cx);
    div()
        .font_family(theme.ui_family())
        .text_size(ScaledText::Sm.size(&theme))
        .text_color(theme.palette(Surface::Window).muted_foreground())
        .child(text.to_owned())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::measure;

    #[gpui::test]
    fn a_note_has_the_height_of_one_text_line(cx: &mut gpui::TestAppContext) {
        let bounds = measure(cx, |_, cx| note_text("No hosts.", cx).into_any_element());
        assert!(f32::from(bounds.size.height) > 0.0 && f32::from(bounds.size.height) < 24.0);
    }
}
