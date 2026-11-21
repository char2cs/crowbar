//! What a dropped branch lands in, in the workspace and folder id space.

pub(crate) enum Container {
    Root,
    Workspace(String),
    Folder(String),
}
