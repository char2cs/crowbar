use crate::frontend::Frontend;
use crate::frontend_asset::FrontendAsset;
#[cfg(target_os = "ios")]
use std::collections::HashMap;
use std::error::Error;
use std::sync::Arc;
use tauri::plugin::{Builder, PluginApi, TauriPlugin};
use tauri::{AppHandle, Asset, Manager, Runtime};

#[cfg(target_os = "ios")]
tauri::ios_plugin_binding!(init_plugin_crowbar_chrome);

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
    register_swift_plugin(api, address)
}

// PLATFORM: tauri hands a command with no Rust handler to the Swift plugin, so there are no Rust commands:
// PLATFORM: tauri payload and channels stay intact, where a Rust wrapper would re-key the payload.
#[cfg(target_os = "ios")]
fn register_swift_plugin<R: Runtime>(
    api: &PluginApi<R, ()>,
    frontend: usize,
) -> Result<(), Box<dyn Error>> {
    let plugin = api.register_ios_plugin(init_plugin_crowbar_chrome)?;
    plugin.run_mobile_plugin::<()>("attachFrontend", HashMap::from([("frontend", frontend)]))?;
    Ok(())
}

#[cfg(not(target_os = "ios"))]
#[expect(
    clippy::unnecessary_wraps,
    reason = "same signature as the iOS registration"
)]
fn register_swift_plugin<R: Runtime>(
    _api: &PluginApi<R, ()>,
    _frontend: usize,
) -> Result<(), Box<dyn Error>> {
    Ok(())
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
