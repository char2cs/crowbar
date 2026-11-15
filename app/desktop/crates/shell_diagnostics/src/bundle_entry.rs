#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct BundleEntry {
    pub(crate) name: String,
    pub(crate) bytes: Vec<u8>,
}
