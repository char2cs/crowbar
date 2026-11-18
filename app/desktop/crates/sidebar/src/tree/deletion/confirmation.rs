//! The last question before a repository or space is deleted.

use crate::tree::deletion::hold::Hold;
use crate::tree::deletion::removal_kind::RemovalKind;
use crate::tree::deletion::removal_target::RemovalTarget;
use crate::tree::deletion::work_at_risk::WorkAtRisk;

const PROJECT_CASCADE: (&str, &str) = (
    "All repositories and workspaces inside it will be deleted.",
    "Every worktree Crowbar created for them is removed from disk. Repositories you imported are unregistered — the folders you originally pointed Crowbar at are left where they are.",
);
const REPO_CASCADE: (&str, &str) = (
    "All workspaces in this repository will be deleted.",
    "Every worktree Crowbar created for them is removed from disk. The folder you originally imported is left where it is.",
);
const WORK_AT_RISK_LEAD: (&str, &str) = (
    "This work exists nowhere else — not on another branch, a remote or a tag.",
    "Deleting removes it for good. Your own checkout is never touched.",
);

/// The dialog shown before a repository or space is deleted, worded by what it removes and
/// whether any work would be lost.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Confirmation {
    /// What the deletion removes once confirmed.
    pub target: RemovalTarget,
    pub(crate) kind: RemovalKind,
    pub(crate) label: String,
    /// Work that exists nowhere else and would be lost; empty when nothing is at risk.
    pub at_risk: Vec<WorkAtRisk>,
}

impl Confirmation {
    pub(crate) fn for_hold(
        hold: &Hold,
        at_risk: Vec<WorkAtRisk>,
    ) -> Self {
        Self {
            target: RemovalTarget::Row(hold.row.clone()),
            kind: hold.kind,
            label: hold.label.clone(),
            at_risk,
        }
    }

    /// The dialog title, naming the noun and label, e.g. "Delete repository “demo”?".
    /// It adds "and lose work" when work is at risk.
    pub fn title(&self) -> String {
        let (noun, label) = (self.kind.noun(), &self.label);
        if self.at_risk.is_empty() {
            return format!("Delete {noun} “{label}”?");
        }
        format!("Delete {noun} “{label}” and lose work?")
    }

    /// The headline and detail sentences under the title.
    /// They warn about lost work when any is at risk, otherwise they describe the cascade.
    pub fn lead(&self) -> (&'static str, &'static str) {
        if !self.at_risk.is_empty() {
            return WORK_AT_RISK_LEAD;
        }
        match self.kind {
            RemovalKind::Project => PROJECT_CASCADE,
            _ => REPO_CASCADE,
        }
    }

    /// The confirm button's text: "Delete <noun>", or "Delete anyway" when work is at risk.
    pub fn confirm_label(&self) -> String {
        if self.at_risk.is_empty() {
            return format!("Delete {}", self.kind.noun());
        }
        "Delete anyway".into()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::shell::ProjectId;
    use crate::shell::RowId;

    fn repo(at_risk: Vec<WorkAtRisk>) -> Confirmation {
        Confirmation {
            target: RemovalTarget::Row(RowId::from("r")),
            kind: RemovalKind::Repo,
            label: "demo".into(),
            at_risk,
        }
    }

    fn risky() -> Confirmation {
        let risk = WorkAtRisk {
            branch: "b".into(),
            uncommitted_files: 1,
            unmerged_commits: 0,
        };
        repo(vec![risk])
    }

    fn space() -> Confirmation {
        let mut confirmation = repo(Vec::new());
        confirmation.kind = RemovalKind::Project;
        confirmation.target = RemovalTarget::Project(ProjectId::from("p"));
        confirmation
    }

    type Case = (&'static str, fn() -> Confirmation, [&'static str; 3]);

    const CASES: &[Case] = &[
        (
            "a clean repository names the cascade",
            || repo(Vec::new()),
            [
                "Delete repository “demo”?",
                "Delete repository",
                "All workspaces",
            ],
        ),
        (
            "work at risk changes every line",
            risky,
            [
                "Delete repository “demo” and lose work?",
                "Delete anyway",
                "This work exists nowhere else",
            ],
        ),
        (
            "a space uses the space cascade",
            space,
            ["Delete space “demo”?", "Delete space", "All repositories"],
        ),
    ];

    #[test]
    fn the_questions_name_the_noun_and_the_risk() {
        for (name, build, [title, button, lead]) in CASES {
            let confirmation = build();
            assert_eq!(confirmation.title(), *title, "{name}: title");
            assert_eq!(confirmation.confirm_label(), *button, "{name}: button");
            assert!(confirmation.lead().0.starts_with(lead), "{name}: lead");
        }
    }
}
