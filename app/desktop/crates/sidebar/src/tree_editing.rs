use crate::editing::Editing;
use crate::lock::Lock;
use crate::new_entry::NewEntry;
use crate::row::Row;
use crate::row_id::RowId;
use crate::row_kind::RowKind;
use crate::tree_state::TreeState;

impl TreeState {
    /// The inline input a tree row is showing, if any.
    pub fn editing(&self) -> &Editing {
        &self.editing
    }

    /// Shows the rename input on `row`, replacing any other inline input.
    /// A locked branch refuses and nothing changes.
    pub fn start_rename(
        &mut self,
        row: &Row,
    ) {
        if row.kind == RowKind::Branch && row.lock == Lock::Locked {
            return;
        }
        self.editing = Editing::Renaming(row.id.clone());
    }

    /// Shows the branch-name input on `row`, replacing any other inline input.
    pub fn start_branching(
        &mut self,
        row: &RowId,
    ) {
        self.editing = Editing::Branching(row.clone());
    }

    /// Shows a draft input under `parent`, unfolding `parent` so the input is visible.
    pub fn start_draft(
        &mut self,
        parent: &RowId,
    ) {
        self.folded.remove(parent);
        self.editing = Editing::Drafting(parent.clone());
    }

    /// Shows the rename input on the file at `path`, replacing any other inline input.
    pub fn start_file_rename(
        &mut self,
        path: String,
    ) {
        self.editing = Editing::RenamingFile(path);
    }

    /// Shows the input that names a new `entry` in the directory `dir`.
    pub fn start_file_create(
        &mut self,
        dir: String,
        entry: NewEntry,
    ) {
        self.editing = Editing::Creating { dir, entry };
    }

    /// Closes whatever inline input is showing.
    pub fn stop_editing(&mut self) {
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
