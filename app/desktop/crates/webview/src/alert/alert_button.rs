use crate::alert::alert_role::AlertRole;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AlertButton {
    pub label: String,
    pub role: AlertRole,
}
