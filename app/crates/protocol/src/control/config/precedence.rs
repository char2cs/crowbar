use serde::{Deserialize, Serialize};

/// Who has the first say on a chord a page also handles.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum Precedence {
    /// The shell runs the command whatever the page did with the key.
    #[default]
    Shell,
    /// The shell runs the command only when the page left the key alone (did not prevent it).
    Page,
}
