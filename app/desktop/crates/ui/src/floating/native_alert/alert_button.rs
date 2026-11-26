use gpui::SharedString;

use crate::floating::native_alert::alert_role::AlertRole;

/// One button of a native message dialog.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AlertButton {
    /// Its text.
    pub label: SharedString,
    /// What it does.
    pub role: AlertRole,
}

impl AlertButton {
    /// A button labelled `label` that does `role`.
    pub fn new(
        label: impl Into<SharedString>,
        role: AlertRole,
    ) -> Self {
        Self {
            label: label.into(),
            role,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_button_keeps_its_words_and_what_it_does() {
        let delete = AlertButton::new("Delete", AlertRole::Destructive);
        assert_eq!(
            (delete.label.as_ref(), delete.role),
            ("Delete", AlertRole::Destructive)
        );
    }
}
