//! The view state of the Files panel: the open dialog, the staged files and the selected row.

use crate::explorer::FileClipboard;
use crate::explorer::FileDialog;

/// What the Files panel shows beyond its rows: a dialog, the staged files and the selection.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct FilesState {
    dialog: Option<FileDialog>,
    pub(crate) clipboard: Option<FileClipboard>,
    pub(crate) selected: Option<String>,
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
    use crate::explorer::FileProperties;

    #[test]
    fn a_dialog_opens_once_and_closes_returning_itself() {
        let mut state = FilesState::default();
        let dialog = FileDialog::Properties(FileProperties::of("a.rs"));
        state.show(dialog.clone());
        assert_eq!(state.dialog(), Some(&dialog));
        assert_eq!(state.close(), Some(dialog));
        assert_eq!(state.dialog(), None);
    }
}
