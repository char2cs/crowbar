//! The error toast shown when the backend refuses something.

use gpui::{IntoElement, ParentElement, Styled, div};
use ui::{Toast, ToastKind};

use crate::spacing;

pub(crate) fn notice_view(text: &str) -> impl IntoElement {
    div()
        .absolute()
        .bottom(spacing::GENEROUS)
        .right(spacing::GENEROUS)
        .child(Toast::new(
            "settings-notice",
            ToastKind::Error,
            text.to_owned(),
        ))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::measure;

    #[gpui::test]
    fn the_notice_is_laid_out_in_a_toast(cx: &mut gpui::TestAppContext) {
        let bounds = measure(cx, |_, _| notice_view("Could not save X."));
        assert!(f32::from(bounds.size.height) >= 0.0);
    }
}
