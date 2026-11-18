use std::time::Instant;

use crate::shell::ProjectId;
use crate::shell::Row;
use crate::shell::RowId;
use crate::tree::Tree;
use crate::tree::deletion::Confirmation;
use crate::tree::deletion::Removals;
use crate::tree::deletion::WorkAtRisk;
use crate::tree::editing::Editing;

impl Tree {
    /// The rows being deleted and the confirmation being asked, if any.
    #[must_use]
    pub fn removals(&self) -> &Removals {
        &self.removals
    }

    /// Hold the row for deletion: a countdown, or a confirmation for repositories.
    /// An inline input open on the row closes once it is held.
    pub fn request_removal(
        &mut self,
        row: &Row,
        now: Instant,
    ) {
        if self.removals.request(row, now) && self.editing.row() == Some(&row.id) {
            self.editing = Editing::Nothing;
        }
    }

    /// Asks to delete the space `project`, named `label`.
    /// A space has no row, so nothing is held; the confirmation alone decides.
    pub fn request_project_removal(
        &mut self,
        project: ProjectId,
        label: &str,
    ) {
        self.removals.request_project(project, label);
    }

    /// Asks again before removing the held `row`, naming the work `at_risk`.
    /// Stops the row's countdown while asking; does nothing when the row is not held.
    pub fn ask_to_discard(
        &mut self,
        row: &RowId,
        at_risk: Vec<WorkAtRisk>,
    ) {
        self.removals.ask_to_discard(row, at_risk);
    }

    /// Undoes the removal of `row`, releasing its hold so the row shows again.
    pub fn keep(
        &mut self,
        row: &RowId,
    ) {
        self.removals.keep(row);
    }

    /// Dismisses the confirmation and keeps the row it was asked about, if any.
    pub fn cancel_removal(&mut self) {
        self.removals.cancel();
    }

    /// Accepts the confirmation and returns it, committing the removal of its row.
    /// Returns `None` when no confirmation was waiting.
    pub fn accept_removal(&mut self) -> Option<Confirmation> {
        self.removals.accept()
    }

    /// The rows whose countdown has run out by `now`, each returned once.
    pub fn take_due_removals(
        &mut self,
        now: Instant,
    ) -> Vec<RowId> {
        self.removals.take_due(now)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::tree::deletion::RemovalTarget;

    #[test]
    fn a_space_removal_waits_for_its_confirmation_and_cancel_drops_it() {
        let mut tree = Tree::default();
        tree.request_project_removal(ProjectId::from("p"), "Demo");
        let asked = tree.removals().confirmation().map(|c| c.target.clone());
        assert_eq!(asked, Some(RemovalTarget::Project(ProjectId::from("p"))));
        tree.cancel_removal();
        assert!(tree.removals().confirmation().is_none(), "cancel drops it");
        tree.request_project_removal(ProjectId::from("p"), "Demo");
        let accepted = tree.accept_removal().map(|c| c.target);
        assert_eq!(accepted, Some(RemovalTarget::Project(ProjectId::from("p"))));
        assert!(tree.accept_removal().is_none(), "accepted once");
    }
}
