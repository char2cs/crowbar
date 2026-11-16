//! What the merge box offers for one [`MergeBox`].

use crate::merge_box::MergeBox;
use crate::merge_work::MergeWork;

const MERGING_LABEL: &str = "Merging…";
const COMMITTING_LABEL: &str = "Committing…";

#[derive(Debug, Clone, Copy)]
pub(crate) struct MergeFacts<'a> {
    merge: &'a MergeBox,
}

fn verb(merge: &MergeBox) -> &'static str {
    if merge.merged {
        return "Merged into";
    }
    "Merge into"
}

impl<'a> MergeFacts<'a> {
    pub(crate) fn of(merge: &'a MergeBox) -> Self {
        Self { merge }
    }

    pub(crate) fn title(self) -> String {
        let verb = verb(self.merge);
        self.merge
            .parent
            .as_ref()
            .map_or_else(|| "Merge".to_owned(), |parent| format!("{verb} {parent}"))
    }

    pub(crate) fn shows_content(self) -> bool {
        !self.merge.merged
    }

    pub(crate) fn offers_commit(self) -> bool {
        !self.merge.uncommitted.is_empty()
    }

    pub(crate) fn commit_label(
        self,
        ticked: usize,
    ) -> String {
        match (self.merge.work == MergeWork::Committing, ticked) {
            (true, _) => COMMITTING_LABEL.to_owned(),
            (false, 1) => "Commit 1 change".to_owned(),
            (false, count) => format!("Commit {count} changes"),
        }
    }

    pub(crate) fn can_commit(
        self,
        ticked: usize,
    ) -> bool {
        self.merge.work == MergeWork::Idle && ticked > 0
    }

    pub(crate) fn confirm_label(self) -> &'static str {
        if self.merge.work == MergeWork::Merging {
            return MERGING_LABEL;
        }
        self.merge.strategy.confirm()
    }

    pub(crate) fn can_merge(self) -> bool {
        !self.merge.merged && self.merge.work == MergeWork::Idle && self.merge.blocked.is_none()
    }

    pub(crate) fn offers_stop(self) -> bool {
        self.merge.working.is_some()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::merge_strategy::MergeStrategy;

    fn open() -> MergeBox {
        MergeBox {
            parent: Some("main".into()),
            ..MergeBox::default()
        }
    }

    fn merged() -> MergeBox {
        MergeBox {
            merged: true,
            ..open()
        }
    }

    fn blocked() -> MergeBox {
        MergeBox {
            blocked: Some("Nothing to merge yet".into()),
            ..open()
        }
    }

    fn merging() -> MergeBox {
        MergeBox {
            work: MergeWork::Merging,
            ..open()
        }
    }

    fn squash() -> MergeBox {
        MergeBox {
            strategy: MergeStrategy::Squash,
            ..open()
        }
    }

    fn uncommitted() -> MergeBox {
        MergeBox {
            uncommitted: vec!["a.rs".into()],
            ..open()
        }
    }

    fn working() -> MergeBox {
        MergeBox {
            working: Some("busy".into()),
            ..open()
        }
    }

    fn busy(work: MergeWork) -> MergeBox {
        MergeBox {
            work,
            ..uncommitted()
        }
    }

    type Case = (
        &'static str,
        fn() -> MergeBox,
        &'static str,
        &'static str,
        [bool; 3],
    );

    const CASES: [Case; 7] = [
        (
            "open",
            open,
            "Merge into main",
            "Create merge commit",
            [true, false, false],
        ),
        (
            "merged",
            merged,
            "Merged into main",
            "Create merge commit",
            [false, false, false],
        ),
        (
            "blocked",
            blocked,
            "Merge into main",
            "Create merge commit",
            [false, false, false],
        ),
        (
            "merging",
            merging,
            "Merge into main",
            "Merging…",
            [false, false, false],
        ),
        (
            "squash",
            squash,
            "Merge into main",
            "Squash & merge",
            [true, false, false],
        ),
        (
            "uncommitted",
            uncommitted,
            "Merge into main",
            "Create merge commit",
            [true, true, false],
        ),
        (
            "working",
            working,
            "Merge into main",
            "Create merge commit",
            [true, false, true],
        ),
    ];

    #[test]
    fn the_box_offers_what_the_daemon_allows() {
        for (name, build, title, label, [can_merge, commit, stop]) in CASES {
            let merge = build();
            let facts = MergeFacts::of(&merge);
            assert_eq!(facts.title(), title, "{name} title");
            assert_eq!(facts.confirm_label(), label, "{name} label");
            assert_eq!(facts.can_merge(), can_merge, "{name} can merge");
            assert_eq!(facts.offers_commit(), commit, "{name} commit");
            assert_eq!(facts.offers_stop(), stop, "{name} stop");
        }
    }

    #[test]
    fn the_commit_button_counts_the_ticked_changes_and_waits_while_committing() {
        type Case = (MergeWork, usize, &'static str, bool);
        const CASES: [Case; 5] = [
            (MergeWork::Idle, 0, "Commit 0 changes", false),
            (MergeWork::Idle, 1, "Commit 1 change", true),
            (MergeWork::Idle, 4, "Commit 4 changes", true),
            (MergeWork::Committing, 4, "Committing…", false),
            (MergeWork::Committing, 0, "Committing…", false),
        ];
        for (work, ticked, label, can) in CASES {
            let merge = busy(work);
            let facts = MergeFacts::of(&merge);
            assert_eq!(facts.commit_label(ticked), label, "{work:?} {ticked}");
            assert_eq!(facts.can_commit(ticked), can, "{work:?} {ticked}");
        }
    }

    #[test]
    fn a_box_without_a_parent_is_titled_plain_merge() {
        assert_eq!(MergeFacts::of(&MergeBox::default()).title(), "Merge");
    }

    #[test]
    fn a_merged_box_hides_its_content() {
        assert!(!MergeFacts::of(&merged()).shows_content());
        assert!(MergeFacts::of(&open()).shows_content());
    }
}
