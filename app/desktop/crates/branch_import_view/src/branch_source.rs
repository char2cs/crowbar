//! What the dialog needs of the data behind it, so the data can be replaced in tests.

use shell_data::{BranchListing, ImportPlan};

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
