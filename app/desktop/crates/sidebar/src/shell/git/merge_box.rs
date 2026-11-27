use crate::shell::git::merge_strategy::MergeStrategy;
use crate::shell::git::merge_work::MergeWork;

#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct MergeBox {
    pub parent: Option<String>,
    pub merged: bool,
    pub strategy: MergeStrategy,
    pub blocked: Option<String>,
    pub uncommitted: Vec<String>,
    pub work: MergeWork,
    pub working: Option<String>,
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
