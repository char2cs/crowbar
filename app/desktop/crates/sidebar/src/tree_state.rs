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

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub(crate) struct TreeState {
    folded: BTreeSet<RowId>,
    editing: Editing,
    tray: RemovalTray,
    confirmation: Option<Confirmation>,
}

impl TreeState {
    pub(crate) fn with_folded(folded: impl IntoIterator<Item = RowId>) -> Self {
        Self {
            folded: folded.into_iter().collect(),
            ..Self::default()
        }
    }

    pub(crate) fn folded(&self) -> impl Iterator<Item = &RowId> {
        self.folded.iter()
    }

    pub(crate) fn toggle_fold(
        &mut self,
        row: &RowId,
    ) {
        if !self.folded.remove(row) {
            self.folded.insert(row.clone());
        }
    }

    pub(crate) fn editing(&self) -> &Editing {
        &self.editing
    }

    pub(crate) fn start_rename(
        &mut self,
        row: &Row,
    ) {
        if row.kind == RowKind::Branch && row.lock == Lock::Locked {
            return;
        }
        self.editing = Editing::Renaming(row.id.clone());
    }

    pub(crate) fn start_branching(
        &mut self,
        row: &RowId,
    ) {
        self.editing = Editing::Branching(row.clone());
    }

    pub(crate) fn start_draft(
        &mut self,
        parent: &RowId,
    ) {
        self.folded.remove(parent);
        self.editing = Editing::Drafting(parent.clone());
    }

    pub(crate) fn stop_editing(&mut self) {
        self.editing = Editing::Nothing;
    }

    pub(crate) fn tray(&self) -> &RemovalTray {
        &self.tray
    }

    pub(crate) fn confirmation(&self) -> Option<&Confirmation> {
        self.confirmation.as_ref()
    }

    pub(crate) fn request_removal(
        &mut self,
        row: &Row,
        now: Instant,
    ) {
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

    pub(crate) fn request_project_removal(
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

    pub(crate) fn cancel_confirmation(&mut self) {
        let Some(confirmation) = self.confirmation.take() else {
            return;
        };
        if let RemovalTarget::Row(row) = confirmation.target {
            self.tray.keep(&row);
        }
    }

    pub(crate) fn accept_confirmation(&mut self) -> Option<Confirmation> {
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

    pub(crate) fn reconcile(
        &mut self,
        rows: &[Row],
    ) {
        let exists = |id: &RowId| rows.iter().any(|row| &row.id == id);
        self.tray.release_gone(exists);
        self.folded.retain(exists);
    }

    pub(crate) fn visible(
        &self,
        rows: &[Row],
    ) -> Vec<VisibleRow> {
        let mut hidden_below: Option<u8> = None;
        rows.iter()
            .enumerate()
            .filter(|(_, row)| !self.is_hidden(row, &mut hidden_below))
            .filter(|(_, row)| !self.is_committed(row))
            .map(|(index, row)| self.entry(index, row))
            .collect()
    }

    fn is_hidden(
        &self,
        row: &Row,
        hidden_below: &mut Option<u8>,
    ) -> bool {
        if hidden_below.is_some_and(|depth| row.depth > depth) {
            return true;
        }
        let hides_children = self.tray.get(&row.id).is_some() || self.folded.contains(&row.id);
        *hidden_below = hides_children.then_some(row.depth);
        false
    }

    fn is_committed(
        &self,
        row: &Row,
    ) -> bool {
        let hold = self.tray.get(&row.id);
        hold.is_some_and(|hold| hold.deadline == Deadline::Committed)
    }

    fn expansion_of(
        &self,
        row: &Row,
    ) -> Expansion {
        if row.nesting == Nesting::Parent && self.folded.contains(&row.id) {
            return Expansion::Collapsed;
        }
        Expansion::Expanded
    }

    fn entry(
        &self,
        index: usize,
        row: &Row,
    ) -> VisibleRow {
        VisibleRow {
            index,
            expansion: self.expansion_of(row),
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
mod tests {
    use std::time::Duration;

    use super::*;
    use crate::lock::Lock;

    fn rows() -> Vec<Row> {
        let mut repo = Row::new("repo", RowKind::RepoHeader, 0, "demo");
        repo.nesting = Nesting::Parent;
        let mut main = Row::new("main", RowKind::Branch, 1, "main");
        main.nesting = Nesting::Parent;
        vec![
            repo,
            main,
            Row::new("chat", RowKind::Chat, 2, "Untitled"),
            Row::new("other", RowKind::Chat, 1, "Other"),
        ]
    }

    fn indices(
        state: &TreeState,
        rows: &[Row],
    ) -> Vec<usize> {
        state.visible(rows).iter().map(|row| row.index).collect()
    }

    type FoldCase = (&'static str, &'static [&'static str], &'static [usize]);

    const FOLD_CASES: &[FoldCase] = &[
        ("all shown", &[], &[0, 1, 2, 3]),
        ("main's chat hidden", &["main"], &[0, 1, 3]),
        ("everything under the repo hidden", &["main", "repo"], &[0]),
        (
            "unfolding restores",
            &["main", "repo", "repo", "main"],
            &[0, 1, 2, 3],
        ),
    ];

    fn toggled(ids: &[&str]) -> TreeState {
        let mut state = TreeState::default();
        for id in ids {
            state.toggle_fold(&RowId::from(*id));
        }
        state
    }

    #[test]
    fn folding_hides_descendants_only() {
        let rows = rows();
        for (name, toggles, expected) in FOLD_CASES {
            let state = toggled(toggles);
            assert_eq!(indices(&state, &rows), *expected, "{name}");
        }
    }

    #[test]
    fn rename_and_branching_exclude_each_other_and_locked_branches_refuse() {
        let rows = rows();
        let mut state = TreeState::default();
        state.start_rename(&rows[2]);
        assert_eq!(
            state.editing(),
            &Editing::Renaming(RowId::from("chat")),
            "renaming"
        );
        state.start_branching(&RowId::from("other"));
        assert_eq!(
            state.editing(),
            &Editing::Branching(RowId::from("other")),
            "branching replaced it"
        );
        let mut locked = rows[1].clone();
        locked.lock = Lock::Locked;
        state.stop_editing();
        state.start_rename(&locked);
        assert_eq!(state.editing(), &Editing::Nothing, "locked branch refuses");
    }

    #[test]
    fn a_draft_unfolds_its_parent() {
        let rows = rows();
        let mut state = TreeState::default();
        state.toggle_fold(&rows[1].id);
        state.start_draft(&rows[1].id);
        assert_eq!(indices(&state, &rows), vec![0, 1, 2, 3], "parent unfolded");
        assert_eq!(
            state.editing(),
            &Editing::Drafting(RowId::from("main")),
            "draft open"
        );
    }

    #[test]
    fn a_chat_removal_counts_down_keeps_its_slot_and_hides_children() {
        let rows = rows();
        let now = Instant::now();
        let mut state = TreeState::default();
        state.request_removal(&rows[1], now);
        let visible = state.visible(&rows);
        assert_eq!(
            visible.iter().map(|row| row.index).collect::<Vec<_>>(),
            vec![0, 1, 3],
            "children hide"
        );
        assert!(
            visible[1].removal.is_some(),
            "the held row is drawn as a countdown"
        );
        state.keep(&rows[1].id);
        assert_eq!(
            indices(&state, &rows),
            vec![0, 1, 2, 3],
            "keep restores exactly"
        );
    }

    #[test]
    fn due_removals_are_sent_and_then_hidden_until_the_tombstone() {
        let rows = rows();
        let now = Instant::now();
        let mut state = TreeState::default();
        state.request_removal(&rows[3], now);
        assert!(
            state.take_due(now + Duration::from_secs(1)).is_empty(),
            "too early"
        );
        let due = state.take_due(now + RemovalTray::DRAIN);
        assert_eq!(due, vec![RowId::from("other")], "sent");
        assert_eq!(
            indices(&state, &rows),
            vec![0, 1, 2],
            "committed row hidden"
        );
        state.reconcile(&rows[..3]);
        assert!(
            state.tray().get(&rows[3].id).is_none(),
            "tombstone releases the hold"
        );
    }

    #[test]
    fn repositories_ask_and_cancelling_brings_the_row_back() {
        let rows = rows();
        let mut state = TreeState::default();
        state.request_removal(&rows[0], Instant::now());
        assert_eq!(
            state.confirmation().map(Confirmation::title),
            Some("Delete repository “demo”?".into()),
            "asks"
        );
        state.cancel_confirmation();
        assert!(
            state.confirmation().is_none() && state.tray().get(&rows[0].id).is_none(),
            "cancelled"
        );
        state.request_removal(&rows[0], Instant::now());
        let accepted = state.accept_confirmation();
        assert_eq!(
            accepted.map(|confirmation| confirmation.target),
            Some(RemovalTarget::Row(RowId::from("repo"))),
            "accepted"
        );
        assert_eq!(
            indices(&state, &rows),
            Vec::<usize>::new(),
            "repo and children hidden after sending"
        );
    }

    #[test]
    fn a_refused_delete_asks_again_naming_the_loss() {
        let rows = rows();
        let mut state = TreeState::default();
        state.request_removal(&rows[2], Instant::now());
        let risk = WorkAtRisk {
            branch: "b".into(),
            uncommitted_files: 2,
            unmerged_commits: 0,
        };
        state.ask_to_discard(&rows[2].id, vec![risk]);
        let title = state.confirmation().map(Confirmation::title);
        assert_eq!(
            title,
            Some("Delete chat “Untitled” and lose work?".into()),
            "second question"
        );
        assert!(!state.tray().is_counting(), "no clock while asking");
    }

    #[test]
    fn locked_branches_cannot_be_removed() {
        let mut locked = rows()[1].clone();
        locked.lock = Lock::Locked;
        let mut state = TreeState::default();
        state.request_removal(&locked, Instant::now());
        assert!(state.tray().get(&locked.id).is_none(), "no hold");
    }

    #[test]
    fn spaces_ask_without_a_row() {
        let mut state = TreeState::default();
        state.request_project_removal(ProjectId::from("p"), "inv");
        assert_eq!(
            state.confirmation().map(Confirmation::title),
            Some("Delete space “inv”?".into()),
            "asks"
        );
    }
}
