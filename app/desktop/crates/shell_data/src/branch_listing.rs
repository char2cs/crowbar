//! The remote branches an import offers, as far as the daemon has answered.

use crate::import_branch::ImportBranch;

/// The branch list: asked for, refused, or read. Protected branches come first.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum BranchListing {
    /// The daemon has not answered yet.
    Loading,
    /// The remote could not be reached.
    Failed,
    /// The branches of the remote.
    Ready(Vec<ImportBranch>),
}
