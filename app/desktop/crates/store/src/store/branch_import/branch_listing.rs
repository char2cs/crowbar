use crate::store::branch_import::import_branch::ImportBranch;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum BranchListing {
    Loading,
    Failed,
    Ready(Vec<ImportBranch>),
}
