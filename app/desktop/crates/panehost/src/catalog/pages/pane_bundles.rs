use std::sync::Arc;

use crate::catalog::pages::bundle_source::BundleSource;
use crate::catalog::pages::extension_bundles::ExtensionBundles;

/// Where pane pages come from: the app's own bundle and the bundles of enabled extensions.
#[derive(Debug, Clone)]
pub struct PaneBundles {
    /// The built-in panes.
    pub core: Arc<dyn BundleSource>,
    /// Extension panes, shared with the host that registers them.
    pub extensions: ExtensionBundles,
}
