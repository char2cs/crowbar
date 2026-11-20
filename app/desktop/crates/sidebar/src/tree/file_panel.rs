use crate::explorer::FileClipboard;
use crate::explorer::FileDialog;
use crate::tree::Tree;
use crate::tree::files_state::FilesState;

impl Tree {
    /// The Files panel's view state: the open dialog, the staged files and the selected row.
    #[must_use]
    pub fn files(&self) -> &FilesState {
        &self.files
    }

    /// Selects the Files row at `path`, or clears the selection with `None`.
    pub fn select_file(
        &mut self,
        path: Option<String>,
    ) {
        self.files.select(path);
    }

    /// Folds the Changes folder at `path`, or unfolds it when folded.
    pub fn toggle_changed_fold(
        &mut self,
        path: String,
    ) {
        self.files.toggle_changed_fold(path);
    }

    /// Stages `clipboard` for the next paste, or clears the staged files with `None`.
    pub fn set_file_clipboard(
        &mut self,
        clipboard: Option<FileClipboard>,
    ) {
        self.files.set_clipboard(clipboard);
    }

    /// Opens the Files `dialog`, replacing any dialog already open.
    pub fn show_file_dialog(
        &mut self,
        dialog: FileDialog,
    ) {
        self.files.show(dialog);
    }

    /// Closes the open Files dialog and returns it, or `None` when none was open.
    pub fn close_file_dialog(&mut self) -> Option<FileDialog> {
        self.files.close()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::shell::ClipMode;

    #[test]
    fn a_selection_and_staged_files_stay_until_cleared_with_none() {
        let mut tree = Tree::default();
        let staged = FileClipboard {
            root: None,
            paths: vec!["a.rs".into()],
            mode: ClipMode::Cut,
        };
        tree.select_file(Some("a.rs".into()));
        tree.set_file_clipboard(Some(staged.clone()));
        assert_eq!(tree.files().selected(), Some("a.rs"));
        assert_eq!(tree.files().clipboard(), Some(&staged));
        tree.select_file(None);
        tree.set_file_clipboard(None);
        assert_eq!(tree.files(), &FilesState::default());
    }
}
