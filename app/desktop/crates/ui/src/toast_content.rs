//! What a toast says.

use gpui::SharedString;

use crate::toast_kind::ToastKind;

/// The tone and text of a toast.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ToastContent {
    pub(crate) kind: ToastKind,
    pub(crate) message: SharedString,
}

impl ToastContent {
    /// A toast of `kind` showing `message`.
    pub fn new(
        kind: ToastKind,
        message: impl Into<SharedString>,
    ) -> Self {
        Self {
            kind,
            message: message.into(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn content_keeps_the_kind_and_the_text() {
        let content = ToastContent::new(ToastKind::Warning, "Low disk");
        assert_eq!(content.kind, ToastKind::Warning);
        assert_eq!(content.message, "Low disk");
    }
}
