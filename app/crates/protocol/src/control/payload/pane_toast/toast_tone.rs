use serde::{Deserialize, Serialize};

/// What a pane's toast reports, which sets its icon and colour.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum ToastTone {
    /// Something worth knowing.
    Info,
    /// An action finished.
    Success,
    /// Something may need attention.
    Warning,
    /// An action failed.
    Error,
    /// An action is still running.
    Loading,
}
