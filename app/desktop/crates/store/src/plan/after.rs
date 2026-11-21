//! What follows a command that succeeded.

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum After {
    /// Nothing: the entity stream shows the result.
    Nothing,
    /// Open the new chat in a pane.
    OpenChat(String),
    /// Show the new space.
    SelectProject(String),
    /// Read the followed chat's changes again.
    ReloadChanges,
    /// Open the name of the folder the daemon answered with for editing.
    NameFolder,
    /// Write the placement answer into this workspace and the siblings it shifted.
    Place(String),
    /// Close the inline input that named this row; a refusal leaves it open over the reason.
    SettleName(sidebar::RowId),
}
