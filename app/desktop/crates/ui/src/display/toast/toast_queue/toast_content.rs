use gpui::SharedString;

use crate::display::toast::toast_kind::ToastKind;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ToastContent {
    pub(crate) kind: ToastKind,
    pub(crate) title: Option<SharedString>,
    pub(crate) message: SharedString,
}

impl ToastContent {
    pub fn new(
        kind: ToastKind,
        message: impl Into<SharedString>,
    ) -> Self {
        Self {
            kind,
            title: None,
            message: message.into(),
        }
    }

    #[must_use]
    pub fn titled(
        mut self,
        title: impl Into<SharedString>,
    ) -> Self {
        self.title = Some(title.into());
        self
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
        assert_eq!(content.title, None, "untitled");
    }

    #[test]
    fn a_title_goes_above_the_message() {
        let content = ToastContent::new(ToastKind::Error, "busy").titled("Couldn't remove x");
        assert_eq!(content.title, Some("Couldn't remove x".into()));
        assert_eq!(content.message, "busy");
    }
}
