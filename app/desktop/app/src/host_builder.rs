use std::path::Path;
use std::sync::Arc;

use panehost::{DirBundle, ExtensionBundles, OriginTable, PaneBundles, PaneHost, PaneProtocol};
use protocol::{DaemonTransport, HostId};
use webview::HostConfig;

use crate::app_options::AppOptions;
use crate::core_panes::register_core_panes;
use crate::error::Result;

pub(crate) const WEBVIEW_DIR: &str = "webview";

const SCHEME: &str = "crowbar";

pub(crate) fn build_host(
    options: &AppOptions,
    (transport, caret): (Arc<dyn DaemonTransport>, webview::CaretReport),
    is_file: impl Fn(&Path) -> bool,
) -> Result<PaneHost> {
    let origins = OriginTable::default();
    let bundles = PaneBundles {
        core: Arc::new(DirBundle::new(
            options.resolve_panes_dir(std::env::current_exe().ok().as_deref(), is_file)?,
        )),
        extensions: ExtensionBundles::default(),
    };
    let protocol = PaneProtocol::new(SCHEME, Arc::clone(&transport), bundles, origins.clone());
    let webviews = webview::Host::native(HostConfig {
        scheme: SCHEME.into(),
        protocol: Arc::new(protocol),
        data_dir: Some(options.state_dir().join(WEBVIEW_DIR)),
    })?;
    webviews.follow_caret(caret)?;
    let config = panehost::Config {
        scheme: SCHEME.into(),
        host: HostId::new("local")?,
        max_idle_slots: 2,
        page_handler: Some(Arc::new(|event| match event {
            webview::PageEvent::Navigating(link) => platform::follow_link(&link, None),
            _ => true,
        })),
    };
    let mut host = PaneHost::new(config, webviews, origins, transport);
    register_core_panes(host.registry_mut(), &options.chat_entry)?;
    host.set_keybindings(keymap::Keymap::defaults().forwarded(&[]))?;
    Ok(host)
}

#[cfg(test)]
mod tests {
    use std::path::PathBuf;

    use super::*;
    use crate::error::Error;
    use crate::test_support::NoDaemon;

    fn options(panes: PathBuf) -> AppOptions {
        AppOptions {
            panes_dir: Some(panes),
            chat_entry: "chat/index.html".into(),
            home: PathBuf::from("/nonexistent-home"),
            daemon_binary: None,
        }
    }

    #[test]
    fn a_missing_bundle_stops_the_host_before_any_web_view_exists() {
        let missing = PathBuf::from("/nonexistent-crowbar-panes");
        let transport = Arc::new(NoDaemon::timing_out());
        let error = build_host(
            &options(missing.clone()),
            (transport, Arc::new(|_| ())),
            |_| false,
        )
        .unwrap_err();
        assert!(
            matches!(&error, Error::PanesMissing(entry) if entry.starts_with(&missing)),
            "{error}"
        );
    }
}
