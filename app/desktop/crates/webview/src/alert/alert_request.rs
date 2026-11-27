use crate::alert::alert_button::AlertButton;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AlertRequest {
    pub title: Option<String>,
    pub message: String,
    pub buttons: Vec<AlertButton>,
}
