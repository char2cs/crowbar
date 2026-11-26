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
