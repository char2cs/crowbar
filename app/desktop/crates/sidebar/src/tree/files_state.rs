use std::collections::BTreeSet;

use crate::explorer::FileClipboard;
use crate::explorer::FileDialog;

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct FilesState {
    dialog: Option<FileDialog>,
    pub(crate) clipboard: Option<FileClipboard>,
    pub(crate) selected: Option<String>,
    changed_folds: BTreeSet<String>,
}

impl FilesState {
    #[must_use]
    pub fn selected(&self) -> Option<&str> {
        self.selected.as_deref()
    }

    pub(crate) fn select(
        &mut self,
        path: Option<String>,
    ) {
        self.selected = path;
    }

    #[must_use]
    pub fn clipboard(&self) -> Option<&FileClipboard> {
        self.clipboard.as_ref()
    }

    pub(crate) fn set_clipboard(
        &mut self,
        clipboard: Option<FileClipboard>,
    ) {
        self.clipboard = clipboard;
    }

    #[must_use]
    pub fn changed_folds(&self) -> &BTreeSet<String> {
        &self.changed_folds
    }

    pub(crate) fn toggle_changed_fold(
        &mut self,
        path: String,
    ) {
        if !self.changed_folds.remove(&path) {
            self.changed_folds.insert(path);
        }
    }

    pub fn dialog(&self) -> Option<&FileDialog> {
        self.dialog.as_ref()
    }

    pub(crate) fn show(
        &mut self,
        dialog: FileDialog,
    ) {
        self.dialog = Some(dialog);
    }

    pub(crate) fn close(&mut self) -> Option<FileDialog> {
        self.dialog.take()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_changes_folder_folds_and_unfolds() {
        let mut state = FilesState::default();
        state.toggle_changed_fold("src".into());
        assert!(state.changed_folds().contains("src"));
        state.toggle_changed_fold("src".into());
        assert!(state.changed_folds().is_empty());
    }
}
