#[derive(Debug, Clone, PartialEq, Eq)]
pub struct BundleAsset {
    pub mime: &'static str,
    pub bytes: Vec<u8>,
}
