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
