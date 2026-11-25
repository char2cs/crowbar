#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct SavedFile {
    pub(crate) path: protocol::DocumentPath,
    pub(crate) text: String,
}
