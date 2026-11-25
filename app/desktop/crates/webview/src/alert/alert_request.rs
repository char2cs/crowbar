use crate::alert::alert_button::AlertButton;

/// A native message dialog to ask over a shell window.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AlertRequest {
    /// Its heading, set in bold above the message; none for a message alone.
    pub title: Option<String>,
    /// What it asks.
    pub message: String,
    /// Its buttons, left to right.
    pub buttons: Vec<AlertButton>,
}
