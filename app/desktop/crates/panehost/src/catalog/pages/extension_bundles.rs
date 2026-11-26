use std::collections::HashMap;
use std::sync::{Arc, RwLock};

use protocol::ExtensionId;

use crate::catalog::pages::bundle_source::BundleSource;

type Sources = Arc<RwLock<HashMap<ExtensionId, Arc<dyn BundleSource>>>>;

/// The pane bundles of enabled extensions, shared between the host that registers them and
/// the protocol router that serves them.
#[derive(Debug, Clone, Default)]
pub struct ExtensionBundles {
    sources: Sources,
}

impl ExtensionBundles {
    /// Serves `extension`'s panes from `source`, replacing an earlier source.
    pub fn insert(
        &self,
        extension: ExtensionId,
        source: Arc<dyn BundleSource>,
    ) {
        self.sources
            .write()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .insert(extension, source);
    }

    /// Stops serving an extension, for example when it is disabled.
    pub fn remove(
        &self,
        extension: &ExtensionId,
    ) {
        self.sources
            .write()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .remove(extension);
    }

    /// The source of an extension's bundle.
    #[must_use]
    pub fn get(
        &self,
        extension: &ExtensionId,
    ) -> Option<Arc<dyn BundleSource>> {
        self.sources
            .read()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .get(extension)
            .cloned()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::catalog::pages::bundle_asset::BundleAsset;

    #[derive(Debug)]
    struct One;

    impl BundleSource for One {
        fn read(
            &self,
            _path: &str,
        ) -> Option<BundleAsset> {
            None
        }
    }

    #[test]
    fn insert_get_remove() {
        let bundles = ExtensionBundles::default();
        let git = ExtensionId::new("git").unwrap();
        assert!(bundles.get(&git).is_none(), "empty");
        bundles.insert(git.clone(), Arc::new(One));
        assert!(
            bundles.clone().get(&git).is_some(),
            "inserted, visible through a clone"
        );
        bundles.remove(&git);
        assert!(bundles.get(&git).is_none(), "removed");
    }
}
