//! The sub-states of a [`Tree`] the views read and drive: the Files panel and the removals.

use crate::tree::Tree;
use crate::tree::deletion::Removals;
use crate::tree::files_state::FilesState;

impl Tree {
    /// The rows being deleted.
    #[must_use]
    pub fn removals(&self) -> &Removals {
        &self.removals
    }

    /// The rows being deleted, for the view to ask, accept, cancel or expire a removal.
    pub fn removals_mut(&mut self) -> &mut Removals {
        &mut self.removals
    }

    /// The Files panel's view state.
    #[must_use]
    pub fn files(&self) -> &FilesState {
        &self.files
    }

    /// The Files panel's view state, for the view to open dialogs, stage files and select rows.
    pub fn files_mut(&mut self) -> &mut FilesState {
        &mut self.files
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_selection_made_through_files_mut_reads_back_through_files() {
        let mut tree = Tree::default();
        tree.files_mut().select(Some("src/main.rs".into()));
        assert_eq!(tree.files().selected(), Some("src/main.rs"));
        assert_eq!(tree.removals(), &Removals::default());
    }
}
