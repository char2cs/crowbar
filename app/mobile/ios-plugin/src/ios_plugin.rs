use crate::frontend::Frontend;
use crate::frontend_asset::FrontendAsset;
use crate::registration::Registration;
use crate::swift_plugin::SwiftPlugin as _;
use std::error::Error;
use std::sync::Arc;
use tauri::plugin::{Builder, PluginApi, TauriPlugin};
use tauri::{AppHandle, Asset, Manager, Runtime};

/// Builds the plugin; on iOS it also starts the Swift chrome that draws over the webview.
pub fn init<R: Runtime>() -> TauriPlugin<R> {
    Builder::new("crowbar-chrome")
        .setup(|app, api| attach(app, &api))
        .build()
}

fn into_frontend_asset(asset: Asset) -> FrontendAsset {
    FrontendAsset {
        bytes: asset.bytes,
        mime_type: asset.mime_type,
    }
}

fn attach<R: Runtime>(
    app: &AppHandle<R>,
    api: &PluginApi<R, ()>,
) -> Result<(), Box<dyn Error>> {
    let dev_url = tauri::is_dev()
        .then(|| app.config().build.dev_url.clone())
        .flatten();
    let resolver = app.asset_resolver();
    let frontend = Arc::new(Frontend::new(dev_url, move |path| {
        resolver.get(path).map(into_frontend_asset)
    }));
    let address = frontend.address();
    app.manage(frontend);
    Registration(api).register(address)
}

#[cfg(test)]
mod tests {
    use super::*;
    use tauri::plugin::Plugin;
    use tauri::test::MockRuntime;

    #[test]
    fn plugin_name_is_the_capability_namespace() {
        let plugin = init::<MockRuntime>();
        assert_eq!(
            Plugin::<MockRuntime>::name(&plugin),
            "crowbar-chrome",
            "capabilities reference the plugin by this name"
        );
    }
}
