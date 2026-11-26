//! The view state of the Files panel: the open dialog, the staged files and the selected row.

use std::collections::BTreeSet;

use crate::explorer::FileClipboard;
use crate::explorer::FileDialog;

/// What the Files panel shows beyond its rows: a dialog, the staged files and the selection.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct FilesState {
    dialog: Option<FileDialog>,
    pub(crate) clipboard: Option<FileClipboard>,
    pub(crate) selected: Option<String>,
    changed_folds: BTreeSet<String>,
}

impl FilesState {
    /// The path of the selected row, if any.
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

    /// The files staged by Copy or Cut, if any.
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

    /// The Changes folders whose contents are hidden, by path.
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

    /// The open dialog, if any.
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
