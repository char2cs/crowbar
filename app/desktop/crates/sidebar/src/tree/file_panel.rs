use crate::explorer::FileClipboard;
use crate::explorer::FileDialog;
use crate::tree::Tree;
use crate::tree::files_state::FilesState;

impl Tree {
    #[must_use]
    pub fn files(&self) -> &FilesState {
        &self.files
    }

    pub fn select_file(
        &mut self,
        path: Option<String>,
    ) {
        self.files.select(path);
    }

    pub fn toggle_changed_fold(
        &mut self,
        path: String,
    ) {
        self.files.toggle_changed_fold(path);
    }

    pub fn set_file_clipboard(
        &mut self,
        clipboard: Option<FileClipboard>,
    ) {
        self.files.set_clipboard(clipboard);
    }

    pub fn show_file_dialog(
        &mut self,
        dialog: FileDialog,
    ) {
        self.files.show(dialog);
    }

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
