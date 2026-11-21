//! What the Changes panel draws.

use std::collections::BTreeMap;

use crate::shell::git::changed_file::ChangedFile;

/// The branch pill and the changed files of the focused repository workspace.
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct Changes {
    /// The checked-out branch.
    pub branch: String,
    /// The branch it merges into, if any.
    pub parent: Option<String>,
    /// Changed files sorted by path.
    pub files: Vec<ChangedFile>,
    /// Unresolved review threads per file path.
    pub threads: BTreeMap<String, u32>,
    /// The workspace's viewed marks: path to the signature the file had when it was marked.
    pub viewed: BTreeMap<String, String>,
    /// Why git could not read the workspace; the panel shows it instead of the tree and merge card.
    pub error: Option<String>,
}
