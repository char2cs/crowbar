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
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("PaneBundles").finish_non_exhaustive()
    }
}

#[cfg(test)]
#[path = "pane_bundles_tests.rs"]
mod tests;
