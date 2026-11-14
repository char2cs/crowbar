/// One file of a pane bundle.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct BundleAsset {
    /// Media type for the `content-type` header.
    pub mime: &'static str,
    /// File content.
    pub bytes: Vec<u8>,
}
