//! The shell-owned view state of the chats tree: folds, inline input and removal.

use std::collections::BTreeSet;
use std::time::Instant;

use crate::confirmation::Confirmation;
use crate::deadline::Deadline;
use crate::editing::Editing;
use crate::expansion::Expansion;
use crate::lock::Lock;
use crate::nesting::Nesting;
use crate::project_id::ProjectId;
use crate::removal_kind::RemovalKind;
use crate::removal_target::RemovalTarget;
use crate::removal_tray::RemovalTray;
use crate::row::Row;
use crate::row_id::RowId;
use crate::row_kind::RowKind;
use crate::visible_row::VisibleRow;
use crate::work_at_risk::WorkAtRisk;

/// Folded rows, the inline input, the removal tray and the open confirmation. The rows themselves
/// stay with the data source: this holds ids only.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct TreeState {
    folded: BTreeSet<RowId>,
    editing: Editing,
    tray: RemovalTray,
    confirmation: Option<Confirmation>,
}

impl TreeState {
    /// A state with the given rows folded, as restored from preferences.
    pub fn with_folded(folded: impl IntoIterator<Item = RowId>) -> Self {
        Self {
            folded: folded.into_iter().collect(),
            ..Self::default()
        }
    }

    /// The folded row ids, for persistence.
    pub fn folded(&self) -> impl Iterator<Item = &RowId> {
        self.folded.iter()
    }

    /// Folds an expanded row and expands a folded one.
    pub fn toggle_fold(&mut self, row: &RowId) {
        if !self.folded.remove(row) {
            self.folded.insert(row.clone());
        }
    }

    /// The inline input in use.
    pub fn editing(&self) -> &Editing {
        &self.editing
    }

    /// Replaces the label of `row` with an input; locked branches refuse. Stops any other input.
    pub fn start_rename(&mut self, row: &Row) {
        if row.kind == RowKind::Branch && row.lock == Lock::Locked {
            return;
        }
        self.editing = Editing::Renaming(row.id.clone());
    }

    /// Turns the chat row into the "make a branch" input. Stops any other input.
    pub fn start_branching(&mut self, row: &RowId) {
        self.editing = Editing::Branching(row.clone());
    }

    /// Opens a naming input for a fork under `parent` and unfolds the parent.
    pub fn start_draft(&mut self, parent: &RowId) {
        self.folded.remove(parent);
        self.editing = Editing::Drafting(parent.clone());
    }

    /// Closes the inline input, confirmed or cancelled.
    pub fn stop_editing(&mut self) {
        self.editing = Editing::Nothing;
    }

    /// The removal tray.
    pub fn tray(&self) -> &RemovalTray {
        &self.tray
    }

    /// The confirmation waiting for an answer.
    pub fn confirmation(&self) -> Option<&Confirmation> {
        self.confirmation.as_ref()
    }

    /// Starts removing `row`: a countdown with Keep for chats, folders and workspaces, the
    /// confirm dialog for a repository. Locked branches refuse.
    pub fn request_removal(&mut self, row: &Row, now: Instant) {
        let Some(kind) = removal_kind(row) else {
            return;
        };
        if self.editing.row() == Some(&row.id) {
            self.editing = Editing::Nothing;
        }
        self.tray
            .hold(row.id.clone(), (kind, row.label.clone()), now);
        if kind.needs_confirmation() {
            self.confirmation = self
                .tray
                .get(&row.id)
                .map(|hold| Confirmation::for_hold(hold, Vec::new()));
        }
    }

    /// Asks before deleting a whole space; spaces are not rows and have no countdown.
    pub fn request_project_removal(&mut self, project: ProjectId, label: &str) {
        self.confirmation = Some(Confirmation {
            target: RemovalTarget::Project(project),
            kind: RemovalKind::Project,
            label: label.to_owned(),
            at_risk: Vec::new(),
        });
    }

    /// The daemon refused to delete `row` over unsaved work: ask again naming the loss.
    pub fn ask_to_discard(&mut self, row: &RowId, at_risk: Vec<WorkAtRisk>) {
        let Some(hold) = self.tray.get(row).cloned() else {
            return;
        };
        self.tray.await_confirmation(row);
        self.confirmation = Some(Confirmation::for_hold(&hold, at_risk));
    }

    /// Cancels the countdown of `row`; it reappears as it was.
    pub fn keep(&mut self, row: &RowId) {
        self.tray.keep(row);
    }

    /// Answers the confirmation with Cancel.
    pub fn cancel_confirmation(&mut self) {
        let Some(confirmation) = self.confirmation.take() else {
            return;
        };
        if let RemovalTarget::Row(row) = confirmation.target {
            self.tray.keep(&row);
        }
    }

    /// Answers with the destructive button and returns what to delete.
    pub fn accept_confirmation(&mut self) -> Option<Confirmation> {
        let confirmation = self.confirmation.take()?;
        if let RemovalTarget::Row(row) = &confirmation.target {
            self.tray.commit(row);
        }
        Some(confirmation)
    }

    /// Sends every countdown that reached `now` and returns the rows to delete.
    pub fn take_due(&mut self, now: Instant) -> Vec<RowId> {
        self.tray.take_due(now)
    }

    /// Drops holds and folds of rows the daemon no longer lists.
    pub fn reconcile(&mut self, rows: &[Row]) {
        let exists = |id: &RowId| rows.iter().any(|row| &row.id == id);
        self.tray.release_gone(exists);
        self.folded.retain(exists);
    }

    /// The rows to draw: descendants of folded and held rows are skipped, committed rows hide.
    pub fn visible(&self, rows: &[Row]) -> Vec<VisibleRow> {
        let mut hidden_below: Option<u8> = None;
        rows.iter()
            .enumerate()
            .filter(|(_, row)| !self.is_hidden(row, &mut hidden_below))
            .filter(|(_, row)| !self.is_committed(row))
            .map(|(index, row)| self.entry(index, row))
            .collect()
    }

    fn is_hidden(&self, row: &Row, hidden_below: &mut Option<u8>) -> bool {
        if hidden_below.is_some_and(|depth| row.depth > depth) {
            return true;
        }
        let hides_children = self.tray.get(&row.id).is_some() || self.folded.contains(&row.id);
        *hidden_below = hides_children.then_some(row.depth);
        false
    }

    fn is_committed(&self, row: &Row) -> bool {
        let hold = self.tray.get(&row.id);
        hold.is_some_and(|hold| hold.deadline == Deadline::Committed)
    }

    fn entry(&self, index: usize, row: &Row) -> VisibleRow {
        let folded = row.nesting == Nesting::Parent && self.folded.contains(&row.id);
        let expansion = if folded {
            Expansion::Collapsed
        } else {
            Expansion::Expanded
        };
        VisibleRow {
            index,
            expansion,
            removal: self.tray.get(&row.id).cloned(),
        }
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
#[path = "tree_state_tests.rs"]
mod tests;
