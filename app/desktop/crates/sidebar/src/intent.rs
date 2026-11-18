//! What a click on a menu entry or row button means.

use crate::clip_mode::ClipMode;
use crate::command::Command;
use crate::new_entry::NewEntry;
use crate::project_id::ProjectId;
use crate::question::Question;
use crate::row_id::RowId;

/// A user action the sidebar handles: a change to its own view state, a question to ask, or a
/// [`Command`] to send to the shell.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Intent {
    /// Open the inline rename input on the row.
    StartRename(RowId),
    /// Open the inline input that names the branch the chat turns into.
    StartBranching(RowId),
    /// Unfold the row and open the inline input that names a fork of it.
    StartDraft(RowId),
    /// Hold the row for deletion: a countdown, or a confirmation for repositories.
    RequestRemoval(RowId),
    /// Ask to confirm the deletion of the space.
    RequestProjectRemoval(ProjectId),
    /// Cancel the row's pending removal and bring it back.
    Keep(RowId),
    /// Fold or unfold the row's children.
    ToggleFold(RowId),
    /// Open the panel that creates a new space.
    StartCreateSpace,
    /// Open the inline rename input on the file at this path.
    StartFileRename(String),
    /// Open the inline input that creates a file or folder.
    StartFileCreate {
        /// The folder the entry is created in.
        dir: String,
        /// Whether a file or a folder is created.
        entry: NewEntry,
    },
    /// Ask to confirm the deletion of a file or folder.
    RequestFileDelete {
        /// The path being deleted.
        path: String,
        /// Whether the path is a folder.
        is_dir: bool,
    },
    /// Show the properties dialog for the file at this path.
    ShowProperties(String),
    /// Put a file on the sidebar's clipboard to paste later.
    StageFile {
        /// The file being staged.
        path: String,
        /// Whether pasting copies or moves it.
        mode: ClipMode,
    },
    /// Paste the staged files into a folder.
    PasteFiles {
        /// The folder they are pasted into.
        dir: String,
    },
    /// Focus the Files panel and select the file at this path.
    SelectFile(String),
    /// Ask a yes-or-no question before sending its command.
    Ask(Question),
    /// Send the command to the shell.
    Send(Command),
}
