//! What a node of the tree stands for.

#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord)]
pub(crate) enum ItemKind {
    /// A grouping folder.
    Folder,
    /// A workspace (a branch row).
    Workspace,
    /// A chat.
    Chat,
}
