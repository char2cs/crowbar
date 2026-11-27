#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RowKind {
    RepoHeader,
    Folder,
    Branch,
    Chat,
}
