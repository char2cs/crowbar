//! The error toast shown when the backend refuses something.

use gpui::{IntoElement, ParentElement, Styled, div, px};
use ui::{Toast, ToastKind};

pub(crate) fn notice_view(text: &str) -> impl IntoElement {
    div()
        .absolute()
        .bottom(px(16.0))
        .right(px(16.0))
        .child(Toast::new(
            "settings-notice",
            ToastKind::Error,
            text.to_owned(),
        ))
}

#[cfg(test)]
#[path = "notice_view_tests.rs"]
mod tests;
