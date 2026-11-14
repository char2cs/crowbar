//! What the Changes panel draws.

use crate::changed_file::ChangedFile;

/// The branch pill and the changed files of the focused repository workspace.
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct Changes {
    /// The checked-out branch.
    pub branch: String,
    /// The branch it merges into, if any.
    pub parent: Option<String>,
    /// Changed files sorted by path.
    pub files: Vec<ChangedFile>,
}
