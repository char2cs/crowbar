//! One remote branch row of the import dialog.

use crate::store::branch_import::availability::Availability;

/// A remote branch and whether it can be selected.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ImportBranch {
    /// The branch name.
    pub name: String,
    /// Whether it may be imported.
    pub availability: Availability,
}
