use std::sync::Arc;

use pane_host::{
    DirBundle, ExtensionBundles, OriginTable, PaneBundles, PaneHost, PaneHostConfig, PaneProtocol,
};
use protocol::{DaemonTransport, HostId};
use webview::{HostConfig, WebViewHost};

use crate::app_options::AppOptions;
use crate::core_panes::register_core_panes;
use crate::error::Result;

const SCHEME: &str = "crowbar";

/// The pane host for the real window: pages come from the pane bundle directory and
/// requests go to the daemon through `transport`.
pub(crate) fn build_host(
    options: &AppOptions,
    transport: Arc<dyn DaemonTransport>,
) -> Result<PaneHost> {
    let origins = OriginTable::default();
    let bundles = PaneBundles {
        core: Arc::new(DirBundle::new(&options.panes_dir)),
        extensions: ExtensionBundles::default(),
    };
    let protocol = PaneProtocol::new(SCHEME, Arc::clone(&transport), bundles, origins.clone());
    let webviews = WebViewHost::native(HostConfig {
        scheme: SCHEME.into(),
        protocol: Arc::new(protocol),
        data_dir: Some(options.state_dir().join("webview")),
    })?;
    let config = PaneHostConfig {
        scheme: SCHEME.into(),
        host: HostId::new("local")?,
        max_idle_slots: 2,
    };
    let mut host = PaneHost::new(config, webviews, transport, origins);
    register_core_panes(host.registry_mut(), &options.chat_entry)?;
    Ok(host)
}
