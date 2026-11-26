//! The rows being deleted: the undo countdown and the confirmation that may precede it.

use std::time::Instant;

use crate::shell::Lock;
use crate::shell::ProjectId;
use crate::shell::Row;
use crate::shell::RowId;
use crate::shell::RowKind;
use crate::tree::deletion::confirmation::Confirmation;
use crate::tree::deletion::removal_kind::RemovalKind;
use crate::tree::deletion::removal_target::RemovalTarget;
use crate::tree::deletion::removal_tray::RemovalTray;
use crate::tree::deletion::work_at_risk::WorkAtRisk;

/// The rows held behind an undo countdown and the confirmation being asked, if any.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct Removals {
    tray: RemovalTray,
    confirmation: Option<Confirmation>,
}

impl Removals {
    /// The rows hidden behind a removal countdown.
    pub fn tray(&self) -> &RemovalTray {
        &self.tray
    }

    /// The confirmation waiting for an answer, if any.
    pub fn confirmation(&self) -> Option<&Confirmation> {
        self.confirmation.as_ref()
    }

    pub(crate) fn request(
        &mut self,
        row: &Row,
        now: Instant,
    ) -> bool {
        let Some(kind) = removal_kind(row) else {
            return false;
        };
        self.tray
            .hold(row.id.clone(), (kind, removal_label(row)), now);
        if kind.needs_confirmation() {
            self.confirmation = self
                .tray
                .get(&row.id)
                .map(|hold| Confirmation::for_hold(hold, Vec::new()));
        }
        true
    }

    pub(crate) fn request_project(
        &mut self,
        project: ProjectId,
        label: &str,
    ) {
        self.confirmation = Some(Confirmation {
            target: RemovalTarget::Project(project),
            kind: RemovalKind::Project,
            label: label.to_owned(),
            at_risk: Vec::new(),
        });
    }

    pub(crate) fn ask_to_discard(
        &mut self,
        row: &RowId,
        at_risk: Vec<WorkAtRisk>,
    ) {
        let Some(hold) = self.tray.get(row).cloned() else {
            return;
        };
        self.tray.await_confirmation(row);
        self.confirmation = Some(Confirmation::for_hold(&hold, at_risk));
    }

    pub(crate) fn keep(
        &mut self,
        row: &RowId,
    ) {
        self.tray.keep(row);
    }

    pub(crate) fn cancel(&mut self) {
        let Some(confirmation) = self.confirmation.take() else {
            return;
        };
        if let RemovalTarget::Row(row) = confirmation.target {
            self.tray.keep(&row);
        }
    }

    pub(crate) fn accept(&mut self) -> Option<Confirmation> {
        let confirmation = self.confirmation.take()?;
        if let RemovalTarget::Row(row) = &confirmation.target {
            self.tray.commit(row);
        }
        Some(confirmation)
    }

    pub(crate) fn take_due(
        &mut self,
        now: Instant,
    ) -> Vec<RowId> {
        self.tray.take_due(now)
    }

    pub(crate) fn release_gone(
        &mut self,
        exists: impl Fn(&RowId) -> bool,
    ) {
        self.tray.release_gone(exists);
    }
}

fn removal_kind(row: &Row) -> Option<RemovalKind> {
    match row.kind {
        RowKind::RepoHeader => Some(RemovalKind::Repo),
        RowKind::Folder => Some(RemovalKind::Folder),
        RowKind::Chat => Some(RemovalKind::Chat),
        RowKind::Branch if row.lock == Lock::Locked => None,
        RowKind::Branch => Some(RemovalKind::Workspace),
    }
}

fn removal_label(row: &Row) -> String {
    match (&row.kind, &row.workspace) {
        (RowKind::Branch, Some(workspace)) => workspace.branch.clone(),
        _ => row.label.clone(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn chat() -> Row {
        Row::new("chat", RowKind::Chat, 2, "Untitled")
    }

    #[test]
    fn a_refused_delete_asks_again_naming_the_loss() {
        let mut removals = Removals::default();
        removals.request(&chat(), Instant::now());
        let risk = WorkAtRisk {
            branch: "b".into(),
            uncommitted_files: 2,
            unmerged_commits: 0,
        };
        removals.ask_to_discard(&chat().id, vec![risk]);
        let title = removals.confirmation().map(Confirmation::title);
        assert_eq!(
            title,
            Some("Delete chat “Untitled” and lose work?".into()),
            "second question"
        );
        assert!(!removals.tray().is_counting(), "no clock while asking");
    }

    #[test]
    fn a_workspace_is_named_by_its_branch() {
        use crate::shell::Workspace;
        use crate::shell::WorkspaceStatus;
        let mut row = Row::new("w", RowKind::Branch, 1, "Dirty chat");
        row.workspace = Some(Workspace {
            status: WorkspaceStatus::New,
            branch: "ws/dirty".into(),
            added: 0,
            deleted: 0,
            held_by_path: None,
        });
        let mut removals = Removals::default();
        removals.request(&row, Instant::now());
        removals.ask_to_discard(&row.id, Vec::new());
        assert_eq!(
            removals.confirmation().map(Confirmation::title),
            Some("Delete workspace “ws/dirty”?".into()),
            "the branch, as develop names it"
        );
    }

    #[test]
    fn what_may_be_removed_and_what_asks_first() {
        type Case = (RowKind, Lock, bool, bool);
        const CASES: [Case; 4] = [
            (RowKind::Chat, Lock::Unlocked, true, false),
            (RowKind::Branch, Lock::Locked, false, false),
            (RowKind::Branch, Lock::Unlocked, true, false),
            (RowKind::RepoHeader, Lock::Unlocked, true, true),
        ];
        for (kind, lock, held, asks) in CASES {
            let mut row = Row::new("r", kind, 1, "row");
            row.lock = lock;
            let mut removals = Removals::default();
            assert_eq!(
                removals.request(&row, Instant::now()),
                held,
                "{kind:?} held"
            );
            assert_eq!(removals.confirmation().is_some(), asks, "{kind:?} asks");
        }
    }
}
