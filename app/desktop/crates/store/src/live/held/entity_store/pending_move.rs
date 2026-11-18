//! A move that waits for the end of a chat's turn.

use serde::Deserialize;

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub(crate) struct PendingMove {
    pub(crate) label: String,
}
