//! The view state of the Files panel: the open dialog, the staged files and the selected row.

use crate::file_clipboard::FileClipboard;
use crate::file_dialog::FileDialog;

/// What the Files panel shows beyond its rows: a dialog, the staged files and the selection.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct FilesState {
    dialog: Option<FileDialog>,
    /// The files staged by Copy or Cut, if any.
    pub clipboard: Option<FileClipboard>,
    /// The path of the selected row, if any.
    pub selected: Option<String>,
}

impl FilesState {
    /// The open dialog, if any.
    pub fn dialog(&self) -> Option<&FileDialog> {
        self.dialog.as_ref()
    }

    /// Opens `dialog`, replacing any dialog already open.
    pub fn show(
        &mut self,
        dialog: FileDialog,
    ) {
        self.dialog = Some(dialog);
    }

    /// Closes the open dialog and returns it, or `None` when none was open.
    pub fn close(&mut self) -> Option<FileDialog> {
        self.dialog.take()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::file_properties::FileProperties;

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
