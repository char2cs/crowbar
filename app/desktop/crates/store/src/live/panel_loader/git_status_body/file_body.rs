//! One file of the daemon's git status.

use serde::Deserialize;

#[derive(Debug, Default, Deserialize)]
#[serde(default)]
pub(crate) struct FileBody {
    pub(crate) path: String,
    pub(crate) status: String,
    pub(crate) staged: bool,
}
