//! One file of the branch summary.

use serde::Deserialize;

#[derive(Debug, Default, Deserialize)]
#[serde(default)]
pub(super) struct SummaryFile {
    pub(super) path: String,
    pub(super) status: String,
    pub(super) additions: u32,
    pub(super) deletions: u32,
}
