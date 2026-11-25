use crate::alert::alert_role::AlertRole;

/// One button of a native message dialog.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AlertButton {
    /// Its text, shown as written (no mnemonic).
    pub label: String,
    /// What it does.
    pub role: AlertRole,
}
