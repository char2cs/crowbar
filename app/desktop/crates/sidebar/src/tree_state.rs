//! The shell-owned view state of the chats tree: folds, inline input and removal.

use std::collections::BTreeSet;
use std::time::Instant;

use crate::deadline::Deadline;
use crate::editing::Editing;
use crate::expansion::Expansion;
use crate::lock::Lock;
use crate::nesting::Nesting;
use crate::removals::Removals;
use crate::row::Row;
use crate::row_id::RowId;
use crate::row_kind::RowKind;
use crate::visible_row::VisibleRow;

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub(crate) struct TreeState {
    folded: BTreeSet<RowId>,
    editing: Editing,
    pub(crate) removals: Removals,
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

    pub(crate) fn request_removal(
        &mut self,
        row: &Row,
        now: Instant,
    ) {
        if self.removals.request(row, now) && self.editing.row() == Some(&row.id) {
            self.editing = Editing::Nothing;
        }
    }

    pub(crate) fn reconcile(
        &mut self,
        rows: &[Row],
    ) {
        let exists = |id: &RowId| rows.iter().any(|row| &row.id == id);
        self.removals.release_gone(exists);
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
        let hides_children =
            self.removals.tray().get(&row.id).is_some() || self.folded.contains(&row.id);
        *hidden_below = hides_children.then_some(row.depth);
        false
    }

    fn is_committed(
        &self,
        row: &Row,
    ) -> bool {
        let hold = self.removals.tray().get(&row.id);
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
            removal: self.removals.tray().get(&row.id).cloned(),
        }
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use super::*;
    use crate::confirmation::Confirmation;
    use crate::removal_target::RemovalTarget;
    use crate::removal_tray::RemovalTray;

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
        state.removals.keep(&rows[1].id);
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
            state
                .removals
                .take_due(now + Duration::from_secs(1))
                .is_empty(),
            "too early"
        );
        let due = state.removals.take_due(now + RemovalTray::DRAIN);
        assert_eq!(due, vec![RowId::from("other")], "sent");
        assert_eq!(
            indices(&state, &rows),
            vec![0, 1, 2],
            "committed row hidden"
        );
        state.reconcile(&rows[..3]);
        assert!(
            state.removals.tray().get(&rows[3].id).is_none(),
            "tombstone releases the hold"
        );
    }

    #[test]
    fn repositories_ask_and_cancelling_brings_the_row_back() {
        let rows = rows();
        let mut state = TreeState::default();
        state.request_removal(&rows[0], Instant::now());
        assert_eq!(
            state.removals.confirmation().map(Confirmation::title),
            Some("Delete repository “demo”?".into()),
            "asks"
        );
        state.removals.cancel();
        assert!(
            state.removals.confirmation().is_none()
                && state.removals.tray().get(&rows[0].id).is_none(),
            "cancelled"
        );
        state.request_removal(&rows[0], Instant::now());
        let accepted = state.removals.accept();
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
}
