use gpui::SharedString;

use crate::floating::native_alert::alert_button::AlertButton;
use crate::floating::native_alert::alert_role::AlertRole;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AlertRequest {
    pub title: Option<SharedString>,
    pub message: SharedString,
    pub buttons: Vec<AlertButton>,
}

impl AlertRequest {
    pub const GO: usize = 1;

    pub fn confirm(
        title: impl Into<SharedString>,
        message: impl Into<SharedString>,
        cancel: impl Into<SharedString>,
        go: AlertButton,
    ) -> Self {
        Self {
            title: Some(title.into()),
            message: message.into(),
            buttons: vec![AlertButton::new(cancel, AlertRole::Cancel), go],
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_confirmation_cancels_first_and_goes_ahead_at_its_go_index() {
        let go = AlertButton::new("Delete", AlertRole::Destructive);
        let request = AlertRequest::confirm("Delete?", "Gone for good.", "Cancel", go.clone());
        let roles: Vec<AlertRole> = request.buttons.iter().map(|button| button.role).collect();
        assert_eq!(roles, [AlertRole::Cancel, AlertRole::Destructive]);
        assert_eq!(request.buttons.get(AlertRequest::GO), Some(&go));
    }
}
