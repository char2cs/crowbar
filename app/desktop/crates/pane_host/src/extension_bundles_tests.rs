use super::*;
use crate::bundle_asset::BundleAsset;

struct One;

impl BundleSource for One {
    fn read(&self, _path: &str) -> Option<BundleAsset> {
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
