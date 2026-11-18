//! The daemon's description of a failure.

use serde::Deserialize;

/// The daemon's description of a failure; a missing field reads as empty.
#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(default)]
pub struct Problem {
    /// The daemon's error code.
    pub code: String,
    /// The daemon's explanation, fit to show.
    pub message: String,
}
