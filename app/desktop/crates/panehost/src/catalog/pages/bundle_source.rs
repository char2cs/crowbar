use crate::catalog::pages::bundle_asset::BundleAsset;

pub trait BundleSource: Send + Sync + 'static {
    fn read(
        &self,
        path: &str,
    ) -> Option<BundleAsset>;
}
