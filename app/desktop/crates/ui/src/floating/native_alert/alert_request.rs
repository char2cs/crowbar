use gpui::SharedString;

use crate::floating::native_alert::alert_button::AlertButton;

/// A native message dialog to ask over a window.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AlertRequest {
    /// Its heading, set above the message; none for a message alone.
    pub title: Option<SharedString>,
    /// What it asks.
    pub message: SharedString,
    /// Its buttons, left to right.
    pub buttons: Vec<AlertButton>,
}
