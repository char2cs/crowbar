use crate::store::branch_import::availability::Availability;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ImportBranch {
    pub name: String,
    pub availability: Availability,
}
