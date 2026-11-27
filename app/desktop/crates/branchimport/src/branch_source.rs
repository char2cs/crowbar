use std::sync::Arc;

use store::{BranchImport, BranchListing, ImportPlan};

pub(crate) trait BranchSource {
    fn listing(&self) -> BranchListing;

    fn plan(
        &self,
        selected: &[String],
    ) -> ImportPlan;

    fn import(
        &self,
        branches: &[String],
        locked: &[String],
    );
}

impl BranchSource for Arc<BranchImport> {
    fn listing(&self) -> BranchListing {
        BranchImport::listing(self)
    }

    fn plan(
        &self,
        selected: &[String],
    ) -> ImportPlan {
        BranchImport::plan(self, selected)
    }

    fn import(
        &self,
        branches: &[String],
        locked: &[String],
    ) {
        BranchImport::import(self, branches, locked);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn needs_a_source<T: BranchSource>() -> usize {
        size_of::<T>()
    }

    #[test]
    fn a_branch_import_is_a_source() {
        assert!(needs_a_source::<Arc<BranchImport>>() > 0);
    }
}
