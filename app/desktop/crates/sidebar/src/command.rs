//! Everything the sidebar asks the rest of the shell to do.

use crate::drop_mode::DropMode;
use crate::host_id::HostId;
use crate::project_id::ProjectId;
use crate::row_id::RowId;

/// A request from the sidebar. The data source decides what happens and answers through the
/// entity stream; the sidebar never guesses the result.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Command {
    /// Open the settings dialog.
    OpenSettings,
    /// Open Settings on the Hosts page.
    OpenHostSettings(HostId),
    /// Show only the spaces of this host and land on its latest space.
    SelectHost(HostId),
    /// Make this space active.
    SelectProject(ProjectId),
    /// Create a space from the staged form.
    CreateSpace {
        /// The trimmed name, or the folder's base name when empty.
        name: String,
        /// The chosen folder.
        folder: String,
        /// The host the space lives on.
        host: HostId,
    },
    /// Delete a space after its confirmation.
    DeleteProject(ProjectId),
    /// Subscribe to the space's chats again.
    RetryChats(ProjectId),
    /// Open the row in a pane.
    OpenRow(RowId),
    /// Rename the row.
    Rename {
        /// The row.
        row: RowId,
        /// The new text.
        name: String,
    },
    /// Send the delete of a held row.
    CommitRemoval(RowId),
    /// Start a chat in the space's home workspace.
    NewThread(ProjectId),
    /// Start a thread under the row.
    Thread(RowId),
    /// Create a fork of the row.
    Fork {
        /// The parent row.
        parent: RowId,
        /// The branch name the user typed, empty for the default.
        name: String,
    },
    /// Turn the chat into a branch.
    MakeBranch {
        /// The chat row.
        row: RowId,
        /// The branch name.
        name: String,
    },
    /// Create a folder under the row, or in the home when absent.
    CreateFolder(Option<RowId>),
    /// Open the Add repository dialog.
    ImportRepo,
    /// Open the Import branches dialog for the repository.
    ImportBranches(RowId),
    /// Start a thread in a terminal.
    TerminalThread(RowId),
    /// Lock the branch.
    Lock(RowId),
    /// Unlock the branch.
    Unlock(RowId),
    /// Detach the checkout that holds the branch.
    Detach(RowId),
    /// Retry provisioning the workspace.
    Retry(RowId),
    /// Dismiss a failed operation row.
    Dismiss(RowId),
    /// Move the row into the container; the keyboard twin of a drop.
    MoveTo {
        /// The row to move.
        row: RowId,
        /// The container.
        target: RowId,
    },
    /// Cancel a move that waits for the end of a turn.
    CancelMove(RowId),
    /// Place a dragged row.
    Drop {
        /// The dragged row.
        row: RowId,
        /// The row under the pointer.
        target: RowId,
        /// Before, after or into.
        mode: DropMode,
    },
    /// Open a file of the Files or Changes panel.
    OpenFile(String),
    /// Fold or unfold a directory of the Files panel.
    ToggleDirectory(String),
}
