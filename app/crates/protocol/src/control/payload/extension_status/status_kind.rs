use serde::{Deserialize, Serialize};

/// State of an extension as the shell shows it.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum StatusKind {
    /// Running normally.
    Running,
    /// Temporarily unreachable; panes show an error in their frame.
    Unavailable,
    /// Turned off; panes show an error in their frame.
    Disabled,
    /// Recovered; the shell re-sends `mount` with the saved state.
    Back,
}
