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
    use crate::shell::Command;
    use crate::shell::RowKind;
    use crate::tree::deletion::RemovalTarget;
    use crate::tree::deletion::RemovalTray;

    fn workspace() -> Row {
        Row::new("ws", RowKind::Branch, 1, "feature/x")
    }

    fn sent(
        tree: &mut Tree,
        row: &Row,
    ) {
        let now = Instant::now();
        tree.request_removal(row, now);
        let due = tree.take_due_removals(now + RemovalTray::DRAIN);
        assert_eq!(due, vec![row.id.clone()], "the countdown sends the delete");
    }

    fn risk() -> WorkAtRisk {
        WorkAtRisk {
            branch: "feature/x".into(),
            uncommitted_files: 0,
            unmerged_commits: 1,
        }
    }

    #[test]
    fn a_refusal_over_work_at_risk_asks_again_and_only_that_answer_discards_it() {
        let (mut tree, row) = (Tree::default(), workspace());
        sent(&mut tree, &row);
        tree.ask_to_discard(&row.id, vec![risk()]);
        let asked = tree.removals().confirmation().cloned();
        assert_eq!(
            asked.as_ref().map(Confirmation::title).as_deref(),
            Some("Delete workspace “feature/x” and lose work?"),
            "asks again"
        );
        assert_eq!(
            asked.map(|c| c.at_risk),
            Some(vec![risk()]),
            "names the work"
        );
        let accepted = tree.accept_removal().map(|c| c.command());
        assert_eq!(
            accepted,
            Some(Command::RemoveDiscardingWork(row.id.clone()))
        );
        assert!(
            tree.visible(&[row]).is_empty(),
            "hidden while it is deleted"
        );
    }

    fn cancel_the_question(
        tree: &mut Tree,
        row: &RowId,
    ) {
        tree.ask_to_discard(row, vec![risk()]);
        tree.cancel_removal();
    }

    type Settle = fn(&mut Tree, &RowId);

    const CASES: [(&str, Settle); 2] =
        [("cancelled", cancel_the_question), ("refused", Tree::keep)];

    #[test]
    fn a_cancelled_question_or_a_refused_delete_shows_the_row_again() {
        for (name, settle) in CASES {
            let (mut tree, row) = (Tree::default(), workspace());
            sent(&mut tree, &row);
            settle(&mut tree, &row.id);
            assert!(
                tree.removals().confirmation().is_none(),
                "{name}: no question"
            );
            let shown = tree.visible(std::slice::from_ref(&row));
            assert_eq!(shown.len(), 1, "{name}: the row is back");
            assert!(shown.iter().all(|entry| entry.removal.is_none()), "{name}");
        }
    }

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
