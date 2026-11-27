pub(crate) enum Container {
    Root,
    Workspace(String),
    Folder(String),
}
