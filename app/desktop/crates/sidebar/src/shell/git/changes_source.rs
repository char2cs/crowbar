//! What the Changes panel draws.

use crate::shell::git::changes::Changes;
use crate::shell::git::merge_box::MergeBox;

/// What the Changes panel draws.
pub trait ChangesSource {
    /// What the Changes panel draws.
    fn changes(&self) -> Changes;
    /// The merge box of the focused workspace, or `None` without a branch workspace.
    fn merge_box(&self) -> Option<MergeBox> {
        None
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixtures::DemoData;

    #[test]
    fn the_demo_workspace_has_changes_and_no_merge_box() {
        let data = DemoData::reference();
        assert_eq!(data.changes().files.len(), 1);
        assert_eq!(data.merge_box(), None);
    }
}
