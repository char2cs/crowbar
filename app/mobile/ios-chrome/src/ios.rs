use tauri::plugin::{Builder, PluginApi};
use tauri::{AppHandle, Runtime};

mod assets;

tauri::ios_plugin_binding!(init_plugin_crowbar_chrome);

// No Rust commands: Tauri hands a plugin command with no Rust handler straight to the Swift
// plugin, payload and channels intact; a Rust wrapper would re-key the payload and lose them.
pub fn attach<R: Runtime>(builder: Builder<R>) -> Builder<R> {
    builder.setup(|app: &AppHandle<R>, api: PluginApi<R, ()>| {
        let dev_url = tauri::is_dev()
            .then(|| app.config().build.dev_url.clone())
            .flatten();
        let resolver = app.asset_resolver();
        assets::install(dev_url, move |path| resolver.get(path));
        api.register_ios_plugin(init_plugin_crowbar_chrome)?;
        Ok(())
    })
}
