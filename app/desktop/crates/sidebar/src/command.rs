//! Everything the sidebar asks the rest of the shell to do.

use crate::after_merge::AfterMerge;
use crate::drop_mode::DropMode;
use crate::file_tree_flag::FileTreeFlag;
use crate::host_id::HostId;
use crate::merge_strategy::MergeStrategy;
use crate::project_id::ProjectId;
use crate::row_id::RowId;
use crate::space_icon::SpaceIcon;

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
    /// Ask for a folder on the host's own disk for the create-space form; the answer goes to
    /// `Sidebar::set_space_folder`.
    ChooseSpaceFolder(HostId),
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
    /// Show the file or directory in the system file manager; the path is workspace-relative.
    RevealFile {
        /// The path relative to the workspace root.
        path: String,
    },
    /// Fold or unfold a directory of the Files panel.
    ToggleDirectory(String),
    /// Create an empty file; the path is workspace-relative.
    CreateFile {
        /// The new file's path.
        path: String,
    },
    /// Create a directory; the path is workspace-relative.
    CreateDirectory {
        /// The new directory's path.
        path: String,
    },
    /// Copy a file or directory next to itself under a free name.
    DuplicateFile {
        /// The path to copy.
        path: String,
    },
    /// Move a file or directory to `new_path`.
    RenameFile {
        /// The current path.
        path: String,
        /// The path after the rename.
        new_path: String,
    },
    /// Delete a file or directory, after the sidebar's confirmation.
    DeleteFile {
        /// The path to delete.
        path: String,
    },
    /// Copy a file or directory to `destination`, which must not exist yet.
    CopyFile {
        /// The path to copy.
        path: String,
        /// The path of the copy.
        destination: String,
    },
    /// Take a folder out of a multi-folder workspace; its files stay on disk.
    RemoveFolderFromWorkspace {
        /// The folder.
        path: String,
    },
    /// Put the file's text on the clipboard, read through the daemon.
    CopyFileContent {
        /// The file.
        path: String,
    },
    /// Reload the listing of a directory; an empty path is the workspace root.
    RefreshDirectory {
        /// The directory.
        path: String,
    },
    /// Open every file of a directory in tabs; an empty path is the workspace root.
    OpenAllFiles {
        /// The directory.
        path: String,
    },
    /// Fold a directory and its subdirectories; an empty path folds the whole tree.
    CollapseDirectory {
        /// The directory.
        path: String,
    },
    /// Open a terminal in a directory; an empty path is the workspace root.
    OpenTerminalAt {
        /// The directory.
        path: String,
    },
    /// Pick a local file and copy it into a directory; an empty path is the workspace root.
    UploadFile {
        /// The destination directory.
        path: String,
    },
    /// Open image generation for a directory.
    GenerateImage {
        /// The directory the image is saved in.
        path: String,
    },
    /// Pick a folder to add to the workspace.
    AddFolderToWorkspace,
    /// Create a space like [`Command::CreateSpace`], then set `icon` on it; a failure to set the icon
    /// does not undo the space.
    CreateSpaceWithIcon {
        /// The trimmed name, or the folder's base name when empty.
        name: String,
        /// The chosen folder.
        folder: String,
        /// The host the space lives on.
        host: HostId,
        /// The icon staged in the form.
        icon: SpaceIcon,
    },
    /// Set the icon of a space; [`SpaceIcon::Default`] resets it.
    SetSpaceIcon {
        /// The space.
        project: ProjectId,
        /// The new icon.
        icon: SpaceIcon,
    },
    /// Remember the strategy the focused workspace merges with.
    SetMergeStrategy(MergeStrategy),
    /// Merge the focused workspace into its parent with the chosen strategy.
    Merge {
        /// Whether the branch and its chats are deleted afterwards.
        after: AfterMerge,
    },
    /// Stop the agents working in the focused workspace, then merge and delete the branch.
    StopAndMerge,
    /// Commit `paths` of the focused workspace with `message`.
    CommitChanges {
        /// The commit message.
        message: String,
        /// The changed files to commit.
        paths: Vec<String>,
    },
    /// Force-end the agent running on the chat `row`, after the sidebar's confirmation.
    KillAgent(RowId),
    /// Switch a filter of the Files panel; the shell writes the setting the flag names.
    SetFileTreeFlag {
        /// The switch.
        flag: FileTreeFlag,
        /// The new state.
        on: bool,
    },
    /// Create `target` next to the env file `path`, with its values emptied.
    CreateEnvTemplate {
        /// The env file the template is made from.
        path: String,
        /// The file name to create, such as `.env.example`.
        target: String,
    },
}
