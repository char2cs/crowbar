//! The view state of the Files panel: the open dialog, the staged files and the selected row.

use crate::file_clipboard::FileClipboard;
use crate::file_dialog::FileDialog;

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub(crate) struct FilesState {
    dialog: Option<FileDialog>,
    pub(crate) clipboard: Option<FileClipboard>,
    pub(crate) selected: Option<String>,
}

impl FilesState {
    pub(crate) fn dialog(&self) -> Option<&FileDialog> {
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
