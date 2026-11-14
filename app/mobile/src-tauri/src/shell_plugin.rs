//! The shell plugin minus its page script. That script cancels every target=_blank
//! click in every webview, browser tabs included, to call `shell.open`, which a
//! remote page is never granted; so a tab's link went nowhere.
use serde_json::Value as JsonValue;
use tauri::ipc::Invoke;
use tauri::plugin::Plugin;
use tauri::webview::PageLoadPayload;
use tauri::{AppHandle, RunEvent, Runtime, Url, Webview, Window};

struct WithoutPageScript<R: Runtime>(Box<dyn Plugin<R>>);

pub fn init<R: Runtime>() -> impl Plugin<R> {
    WithoutPageScript(Box::new(tauri_plugin_shell::init()))
}

impl<R: Runtime> Plugin<R> for WithoutPageScript<R> {
    fn name(&self) -> &'static str {
        self.0.name()
    }

    fn initialize(
        &mut self,
        app: &AppHandle<R>,
        config: JsonValue,
    ) -> Result<(), Box<dyn std::error::Error>> {
        self.0.initialize(app, config)
    }

    fn window_created(&mut self, window: Window<R>) {
        self.0.window_created(window)
    }

    fn webview_created(&mut self, webview: Webview<R>) {
        self.0.webview_created(webview)
    }

    fn on_navigation(&mut self, webview: &Webview<R>, url: &Url) -> bool {
        self.0.on_navigation(webview, url)
    }

    fn on_page_load(&mut self, webview: &Webview<R>, payload: &PageLoadPayload<'_>) {
        self.0.on_page_load(webview, payload)
    }

    fn on_event(&mut self, app: &AppHandle<R>, event: &RunEvent) {
        self.0.on_event(app, event)
    }

    fn extend_api(&mut self, invoke: Invoke<R>) -> bool {
        self.0.extend_api(invoke)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tauri::test::MockRuntime;

    #[test]
    fn pages_get_no_shell_script_and_the_commands_keep_their_plugin_name() {
        let plugin = init::<MockRuntime>();
        assert_eq!(plugin.name(), "shell");
        assert_eq!(plugin.initialization_script(), None);
        assert!(plugin.initialization_script_2().is_none());
    }
}
