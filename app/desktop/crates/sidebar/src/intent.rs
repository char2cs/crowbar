//! What a click on a menu entry or row button means.

use crate::command::Command;
use crate::project_id::ProjectId;
use crate::row_id::RowId;

/// Either a change of the sidebar's own view state or a command for the shell.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Intent {
    /// Turn the row's label into an input.
    StartRename(RowId),
    /// Turn the chat row into the "make a branch" input.
    StartBranching(RowId),
    /// Open a naming input for a fork of the row.
    StartDraft(RowId),
    /// Hide the row behind a countdown, or ask first for a repository.
    RequestRemoval(RowId),
    /// Ask before deleting a space.
    RequestProjectRemoval(ProjectId),
    /// Cancel a countdown.
    Keep(RowId),
    /// Fold or unfold a parent.
    ToggleFold(RowId),
    /// Replace the chats with the create-space panel.
    StartCreateSpace,
    /// Forward to the shell.
    Send(Command),
}
