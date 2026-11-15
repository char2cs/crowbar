use tauri::plugin::{Builder, TauriPlugin};
use tauri::Runtime;

const CROWBAR_BOOTSTRAP: &str = include_str!("init_scripts/crowbar_bootstrap.js");

#[cfg(debug_assertions)]
const WEBVIEW_CONSOLE_FORWARD: &str = include_str!("init_scripts/webview_console_forward.js");

pub(crate) fn bootstrap<R: Runtime>() -> TauriPlugin<R> {
    Builder::<R, ()>::new("crowbar-bootstrap")
        .js_init_script(CROWBAR_BOOTSTRAP.to_owned())
        .build()
}

#[cfg(debug_assertions)]
pub(crate) fn console_forward<R: Runtime>() -> TauriPlugin<R> {
    Builder::<R, ()>::new("webview-console")
        .js_init_script(WEBVIEW_CONSOLE_FORWARD.to_owned())
        .build()
}

#[cfg(test)]
mod tests {
    use super::*;
    use tauri::plugin::Plugin;
    use tauri::test::MockRuntime;

    #[test]
    fn each_plugin_registers_under_its_own_name() {
        let bootstrap = bootstrap::<MockRuntime>();
        let forward = console_forward::<MockRuntime>();
        let names = [
            Plugin::<MockRuntime>::name(&bootstrap),
            Plugin::<MockRuntime>::name(&forward),
        ];
        assert_eq!(names, ["crowbar-bootstrap", "webview-console"]);
    }
}
