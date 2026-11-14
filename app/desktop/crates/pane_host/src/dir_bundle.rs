use std::path::{Component, Path, PathBuf};

use crate::bundle_asset::BundleAsset;
use crate::bundle_source::BundleSource;
use crate::mime::mime_for;

/// Serves a bundle from a directory. Paths that would leave the directory are not found.
#[derive(Debug, Clone)]
pub struct DirBundle {
    root: PathBuf,
}

impl DirBundle {
    /// A bundle rooted at `root`.
    #[must_use]
    pub fn new(root: impl Into<PathBuf>) -> Self {
        Self { root: root.into() }
    }

    fn resolve(&self, path: &str) -> Option<PathBuf> {
        let relative = if path.is_empty() || path.ends_with('/') {
            format!("{path}index.html")
        } else {
            path.to_owned()
        };
        let confined = Path::new(&relative)
            .components()
            .all(|component| matches!(component, Component::Normal(_)));
        confined.then(|| self.root.join(relative))
    }
}

impl BundleSource for DirBundle {
    fn read(&self, path: &str) -> Option<BundleAsset> {
        let file = self.resolve(path)?;
        let bytes = std::fs::read(&file).ok()?;
        Some(BundleAsset {
            mime: mime_for(&file.to_string_lossy()),
            bytes,
        })
    }
}

#[cfg(test)]
#[path = "dir_bundle_tests.rs"]
mod tests;
