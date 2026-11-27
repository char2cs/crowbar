use crate::shell::Lock;
use crate::shell::NewEntry;
use crate::shell::ProjectId;
use crate::shell::Row;
use crate::shell::RowId;
use crate::shell::RowKind;
use crate::tree::Tree;
use crate::tree::editing::Editing;

impl Tree {
    pub fn editing(&self) -> &Editing {
        &self.editing
    }

    pub fn start_rename(
        &mut self,
        row: &Row,
    ) {
        if row.kind == RowKind::Branch && row.lock == Lock::Locked {
            return;
        }
        self.editing = Editing::Renaming(row.id.clone());
    }

    pub fn start_naming(
        &mut self,
        row: &RowId,
    ) {
        self.editing = Editing::Renaming(row.clone());
    }

    pub fn start_branching(
        &mut self,
        row: &RowId,
    ) {
        self.editing = Editing::Branching(row.clone());
    }

    pub fn start_draft(
        &mut self,
        parent: &RowId,
    ) {
        self.folded.remove(parent);
        self.editing = Editing::Drafting(parent.clone());
    }

    pub fn start_file_rename(
        &mut self,
        path: String,
    ) {
        self.editing = Editing::RenamingFile(path);
    }

    pub fn start_project_rename(
        &mut self,
        project: ProjectId,
    ) {
        self.editing = Editing::RenamingProject(project);
    }

    pub fn start_file_create(
        &mut self,
        dir: String,
        entry: NewEntry,
    ) {
        self.editing = Editing::Creating { dir, entry };
    }

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
        let mut state = Tree::default();
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
