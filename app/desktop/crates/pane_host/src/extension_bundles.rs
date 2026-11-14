use std::collections::HashMap;
use std::sync::{Arc, RwLock};

use protocol::ExtensionId;

use crate::bundle_source::BundleSource;

/// The pane bundles of enabled extensions, shared between the host that registers them and
/// the protocol router that serves them.
#[derive(Clone, Default)]
pub struct ExtensionBundles {
    sources: Arc<RwLock<HashMap<ExtensionId, Arc<dyn BundleSource>>>>,
}

impl std::fmt::Debug for ExtensionBundles {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("ExtensionBundles").finish_non_exhaustive()
    }
}

impl ExtensionBundles {
    /// Serves `extension`'s panes from `source`, replacing an earlier source.
    pub fn insert(&self, extension: ExtensionId, source: Arc<dyn BundleSource>) {
        self.sources
            .write()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .insert(extension, source);
    }

    /// Stops serving an extension, for example when it is disabled.
    pub fn remove(&self, extension: &ExtensionId) {
        self.sources
            .write()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .remove(extension);
    }

    /// The source of an extension's bundle.
    #[must_use]
    pub fn get(&self, extension: &ExtensionId) -> Option<Arc<dyn BundleSource>> {
        self.sources
            .read()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .get(extension)
            .cloned()
    }
}

#[cfg(test)]
#[path = "extension_bundles_tests.rs"]
mod tests;
