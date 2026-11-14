use crate::bundle_asset::BundleAsset;

/// Where the built files of a pane bundle come from: a directory on disk in development and
/// installs, possibly an embedded archive later.
pub trait BundleSource: Send + Sync + 'static {
    /// Reads a file by its path inside the bundle; `None` when it does not exist. The path has
    /// no leading slash and may be empty, which names the bundle's `index.html`.
    fn read(&self, path: &str) -> Option<BundleAsset>;
}
