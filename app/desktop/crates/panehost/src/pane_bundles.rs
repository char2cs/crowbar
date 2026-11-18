use std::sync::Arc;

use crate::bundle_source::BundleSource;
use crate::extension_bundles::ExtensionBundles;

/// Where pane pages come from: the app's own bundle and the bundles of enabled extensions.
#[derive(Clone)]
pub struct PaneBundles {
    /// The built-in panes.
    pub core: Arc<dyn BundleSource>,
    /// Extension panes, shared with the host that registers them.
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
    use crate::bundle_asset::BundleAsset;

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
