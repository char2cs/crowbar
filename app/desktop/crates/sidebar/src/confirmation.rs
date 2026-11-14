//! The last question before a repository or space is deleted.

use crate::hold::Hold;
use crate::removal_kind::RemovalKind;
use crate::removal_target::RemovalTarget;
use crate::work_at_risk::WorkAtRisk;

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

/// A pending delete confirmation, with the unsaved work a refusal named.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Confirmation {
    /// What gets deleted.
    pub target: RemovalTarget,
    /// What kind of thing it is.
    pub kind: RemovalKind,
    /// Its name.
    pub label: String,
    /// Work lost by deleting; empty for the first question.
    pub at_risk: Vec<WorkAtRisk>,
}

impl Confirmation {
    pub(crate) fn for_hold(hold: &Hold, at_risk: Vec<WorkAtRisk>) -> Self {
        Self {
            target: RemovalTarget::Row(hold.row.clone()),
            kind: hold.kind,
            label: hold.label.clone(),
            at_risk,
        }
    }

    /// `Delete repository “demo”?`, or `… and lose work?` when work is at risk.
    pub fn title(&self) -> String {
        let (noun, label) = (self.kind.noun(), &self.label);
        if self.at_risk.is_empty() {
            return format!("Delete {noun} “{label}”?");
        }
        format!("Delete {noun} “{label}” and lose work?")
    }

    /// The bold lead and the rest of the description.
    pub fn lead(&self) -> (&'static str, &'static str) {
        if !self.at_risk.is_empty() {
            return WORK_AT_RISK_LEAD;
        }
        match self.kind {
            RemovalKind::Project => PROJECT_CASCADE,
            _ => REPO_CASCADE,
        }
    }

    /// `Delete repository`, or `Delete anyway` when work is at risk.
    pub fn confirm_label(&self) -> String {
        if self.at_risk.is_empty() {
            return format!("Delete {}", self.kind.noun());
        }
        "Delete anyway".into()
    }
}

#[cfg(test)]
#[path = "confirmation_tests.rs"]
mod tests;
