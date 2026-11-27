use std::collections::HashMap;
use std::sync::{Arc, RwLock};

use protocol::ExtensionId;

use crate::catalog::pages::bundle_source::BundleSource;

type Sources = Arc<RwLock<HashMap<ExtensionId, Arc<dyn BundleSource>>>>;

#[derive(Clone, Default)]
pub struct ExtensionBundles {
    sources: Sources,
}

impl std::fmt::Debug for ExtensionBundles {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("ExtensionBundles").finish_non_exhaustive()
    }
}

impl ExtensionBundles {
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

    pub fn remove(
        &self,
        extension: &ExtensionId,
    ) {
        self.sources
            .write()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .remove(extension);
    }

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
