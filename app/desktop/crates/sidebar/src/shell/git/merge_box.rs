//! What the Changes panel's merge box shows.

use crate::shell::git::merge_strategy::MergeStrategy;
use crate::shell::git::merge_work::MergeWork;

/// The merge box of the focused workspace; the shell has already decided why merging is blocked.
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct MergeBox {
    /// The branch it merges into; the title reads "Merge into `<parent>`".
    pub parent: Option<String>,
    /// The branch has landed in its parent: the title reads "Merged into `<parent>`" and no controls show.
    pub merged: bool,
    /// The strategy chosen for this workspace.
    pub strategy: MergeStrategy,
    /// Why merging is blocked, in the words to show, or `None` when it is allowed.
    pub blocked: Option<String>,
    /// Uncommitted files that block the merge: the commit form lists them.
    pub uncommitted: Vec<String>,
    /// The request the box is waiting on.
    pub work: MergeWork,
    /// The daemon refused the merge because an agent is mid-turn: "Stop and merge" answers it.
    pub working: Option<String>,
    /// The last failure, to show in the destructive line.
    pub error: Option<String>,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_fresh_box_has_nothing_blocked_running_or_failed() {
        let merge = MergeBox::default();
        assert_eq!(merge.strategy, MergeStrategy::Merge);
        assert!(merge.blocked.is_none() && merge.working.is_none() && merge.error.is_none());
        assert!(!merge.merged && merge.work == MergeWork::Idle);
        assert_eq!(merge.uncommitted.len(), 0);
    }
}
