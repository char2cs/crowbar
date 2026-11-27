#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord)]
pub(crate) enum ItemKind {
    Folder,
    Workspace,
    Chat,
    Repo,
}
