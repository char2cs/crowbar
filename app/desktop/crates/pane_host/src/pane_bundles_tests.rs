use std::sync::Arc;

use super::*;
use crate::bundle_asset::BundleAsset;

struct Core;

impl BundleSource for Core {
    fn read(&self, _path: &str) -> Option<BundleAsset> {
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
