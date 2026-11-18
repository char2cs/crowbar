//! What the merge box offers for one [`MergeBox`].

use crate::shell::git::merge_box::MergeBox;
use crate::shell::git::merge_work::MergeWork;

const MERGING_LABEL: &str = "Merging…";
const COMMITTING_LABEL: &str = "Committing…";

/// The titles, labels and allowed actions of the merge box, read from one [`MergeBox`].
#[derive(Debug, Clone, Copy)]
pub struct MergeFacts<'a> {
    merge: &'a MergeBox,
}

fn verb(merge: &MergeBox) -> &'static str {
    if merge.merged {
        return "Merged into";
    }
    "Merge into"
}

impl<'a> MergeFacts<'a> {
    /// The facts of this merge box.
    pub fn of(merge: &'a MergeBox) -> Self {
        Self { merge }
    }

    /// "Merge into `<parent>`", "Merged into `<parent>`" once landed, or plain "Merge" when
    /// the box has no parent.
    pub fn title(self) -> String {
        let verb = verb(self.merge);
        self.merge
            .parent
            .as_ref()
            .map_or_else(|| "Merge".to_owned(), |parent| format!("{verb} {parent}"))
    }

    /// Whether the box shows its controls; a merged box shows only its title.
    pub fn shows_content(self) -> bool {
        !self.merge.merged
    }

    /// Whether the commit form shows, because uncommitted files block the merge.
    pub fn offers_commit(self) -> bool {
        !self.merge.uncommitted.is_empty()
    }

    /// The commit button's text for `ticked` selected files, e.g. "Commit 2 changes".
    /// It reads "Committing…" while the commit runs.
    pub fn commit_label(
        self,
        ticked: usize,
    ) -> String {
        match (self.merge.work == MergeWork::Committing, ticked) {
            (true, _) => COMMITTING_LABEL.to_owned(),
            (false, 1) => "Commit 1 change".to_owned(),
            (false, count) => format!("Commit {count} changes"),
        }
    }

    /// Whether the commit button is enabled: nothing is running and at least one file is ticked.
    pub fn can_commit(
        self,
        ticked: usize,
    ) -> bool {
        self.merge.work == MergeWork::Idle && ticked > 0
    }

    /// The merge button's text: the chosen strategy's verb, or "Merging…" while a merge runs.
    pub fn confirm_label(self) -> &'static str {
        if self.merge.work == MergeWork::Merging {
            return MERGING_LABEL;
        }
        self.merge.strategy.confirm()
    }

    /// Whether the merge button is enabled: not yet merged, nothing running and nothing
    /// blocking.
    pub fn can_merge(self) -> bool {
        !self.merge.merged && self.merge.work == MergeWork::Idle && self.merge.blocked.is_none()
    }

    /// Whether "Stop and merge" shows, because the daemon refused the merge while an agent
    /// is mid-turn.
    pub fn offers_stop(self) -> bool {
        self.merge.working.is_some()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::shell::git::merge_strategy::MergeStrategy;

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
