use crate::editing::Editing;
use crate::lock::Lock;
use crate::new_entry::NewEntry;
use crate::row::Row;
use crate::row_id::RowId;
use crate::row_kind::RowKind;
use crate::tree_state::TreeState;

impl TreeState {
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

    pub(crate) fn start_file_rename(
        &mut self,
        path: String,
    ) {
        self.editing = Editing::RenamingFile(path);
    }

    pub(crate) fn start_file_create(
        &mut self,
        dir: String,
        entry: NewEntry,
    ) {
        self.editing = Editing::Creating { dir, entry };
    }

    pub(crate) fn stop_editing(&mut self) {
        self.editing = Editing::Nothing;
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rename_and_branching_exclude_each_other_and_locked_branches_refuse() {
        let chat = Row::new("chat", RowKind::Chat, 2, "Untitled");
        let mut locked = Row::new("main", RowKind::Branch, 1, "main");
        locked.lock = Lock::Locked;
        let mut state = TreeState::default();
        state.start_rename(&chat);
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
        state.stop_editing();
        state.start_rename(&locked);
        assert_eq!(state.editing(), &Editing::Nothing, "locked branch refuses");
    }
}
