use crate::frontend::Frontend;
use crate::frontend_asset::FrontendAsset;
use std::error::Error;
use tauri::plugin::{Builder, PluginApi, TauriPlugin};
use tauri::{AppHandle, Runtime};

#[cfg(target_os = "ios")]
tauri::ios_plugin_binding!(init_plugin_crowbar_chrome);

/// Builds the plugin; on iOS it also starts the Swift chrome that draws over the webview.
pub fn init<R: Runtime>() -> TauriPlugin<R> {
    Builder::new("crowbar-chrome")
        .setup(|app, api| attach(app, &api))
        .build()
}

fn attach<R: Runtime>(app: &AppHandle<R>, api: &PluginApi<R, ()>) -> Result<(), Box<dyn Error>> {
    let dev_url = tauri::is_dev()
        .then(|| app.config().build.dev_url.clone())
        .flatten();
    let resolver = app.asset_resolver();
    Frontend::install(dev_url, move |path| {
        resolver.get(path).map(|asset| FrontendAsset {
            bytes: asset.bytes,
            mime_type: asset.mime_type,
        })
    });
    register_swift_plugin(api)
}

// PLATFORM: no Rust commands; Tauri hands a command with no Rust handler to the Swift plugin,
// PLATFORM: payload and channels intact, where a Rust wrapper would re-key the payload.
#[cfg(target_os = "ios")]
fn register_swift_plugin<R: Runtime>(api: &PluginApi<R, ()>) -> Result<(), Box<dyn Error>> {
    api.register_ios_plugin(init_plugin_crowbar_chrome)?;
    Ok(())
}

#[cfg(not(target_os = "ios"))]
#[expect(clippy::unnecessary_wraps, reason = "same signature as the iOS registration")]
fn register_swift_plugin<R: Runtime>(_api: &PluginApi<R, ()>) -> Result<(), Box<dyn Error>> {
    Ok(())
}

#[cfg(test)]
#[path = "tauri_plugin_crowbar_chrome_tests.rs"]
mod tests;
