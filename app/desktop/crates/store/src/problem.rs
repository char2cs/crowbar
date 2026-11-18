//! The daemon's description of a failure.

use serde::Deserialize;

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(default)]
pub(crate) struct Problem {
    pub(crate) code: String,
    pub(crate) message: String,
}
