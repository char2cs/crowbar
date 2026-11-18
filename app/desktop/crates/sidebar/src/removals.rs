//! The rows being deleted: the undo countdown and the confirmation that may precede it.

use std::time::Instant;

use crate::confirmation::Confirmation;
use crate::lock::Lock;
use crate::project_id::ProjectId;
use crate::removal_kind::RemovalKind;
use crate::removal_target::RemovalTarget;
use crate::removal_tray::RemovalTray;
use crate::row::Row;
use crate::row_id::RowId;
use crate::row_kind::RowKind;
use crate::work_at_risk::WorkAtRisk;

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
            .hold(row.id.clone(), (kind, row.label.clone()), now);
        if kind.needs_confirmation() {
            self.confirmation = self
                .tray
                .get(&row.id)
                .map(|hold| Confirmation::for_hold(hold, Vec::new()));
        }
        true
    }

    /// Asks to delete the space `project`, named `label`.
    /// A space has no row, so nothing is held; the confirmation alone decides.
    pub fn request_project(
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

    /// Asks again before removing the held `row`, naming the work `at_risk`.
    /// Stops the row's countdown while asking; does nothing when the row is not held.
    pub fn ask_to_discard(
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

    /// Undoes the removal of `row`, releasing its hold so the row shows again.
    pub fn keep(
        &mut self,
        row: &RowId,
    ) {
        self.tray.keep(row);
    }

    /// Dismisses the confirmation and keeps the row it was asked about, if any.
    pub fn cancel(&mut self) {
        let Some(confirmation) = self.confirmation.take() else {
            return;
        };
        if let RemovalTarget::Row(row) = confirmation.target {
            self.tray.keep(&row);
        }
    }

    /// Accepts the confirmation and returns it, committing the removal of its row.
    /// Returns `None` when no confirmation was waiting.
    pub fn accept(&mut self) -> Option<Confirmation> {
        let confirmation = self.confirmation.take()?;
        if let RemovalTarget::Row(row) = &confirmation.target {
            self.tray.commit(row);
        }
        Some(confirmation)
    }

    /// The rows whose countdown has run out by `now`, each returned once.
    pub fn take_due(
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

    #[test]
    fn spaces_ask_without_a_row() {
        let mut removals = Removals::default();
        removals.request_project(ProjectId::from("p"), "inv");
        assert_eq!(
            removals.confirmation().map(Confirmation::title),
            Some("Delete space “inv”?".into()),
            "asks"
        );
    }
}
