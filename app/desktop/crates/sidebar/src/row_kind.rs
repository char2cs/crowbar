//! The kinds of rows in the chats tree.

/// What a tree row stands for.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RowKind {
    /// The repository (or project home) header; hosts the repo menu and icon.
    RepoHeader,
    /// A folder grouping chats and branches.
    Folder,
    /// A branch, possibly with a workspace.
    Branch,
    /// A chat.
    Chat,
}
