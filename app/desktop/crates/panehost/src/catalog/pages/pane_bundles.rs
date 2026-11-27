use std::sync::Arc;

use crate::catalog::pages::bundle_source::BundleSource;
use crate::catalog::pages::extension_bundles::ExtensionBundles;

#[derive(Clone)]
pub struct PaneBundles {
    pub core: Arc<dyn BundleSource>,
    pub extensions: ExtensionBundles,
}

impl std::fmt::Debug for PaneBundles {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("PaneBundles").finish_non_exhaustive()
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;
    use crate::catalog::pages::bundle_asset::BundleAsset;

    struct Core;

    impl BundleSource for Core {
        fn read(
            &self,
            _path: &str,
        ) -> Option<BundleAsset> {
            None
        }
    }

    #[test]
    fn debug_names_the_type_and_nothing_else() {
        let bundles = PaneBundles {
            core: Arc::new(Core),
            extensions: ExtensionBundles::default(),
        };
        assert_eq!(format!("{bundles:?}"), "PaneBundles { .. }", "opaque");
    }
}
