use log::LevelFilter;
use tauri::plugin::TauriPlugin;
use tauri::Runtime;

const CHATTY_TARGETS: [&str; 5] = ["tokio_tungstenite", "hyper", "hyper_util", "reqwest", "mio"];
const TOKEN_BEARING_TARGETS: [&str; 1] = ["tungstenite"];

fn capped(
    builder: tauri_plugin_log::Builder,
    targets: &[&'static str],
) -> tauri_plugin_log::Builder {
    targets.iter().fold(builder, |builder, target| {
        builder.level_for(*target, LevelFilter::Warn)
    })
}

pub(crate) fn plugin<R: Runtime>() -> TauriPlugin<R> {
    let builder = tauri_plugin_log::Builder::new().level(LevelFilter::Info);
    let builder = capped(builder, &CHATTY_TARGETS);
    capped(builder, &TOKEN_BEARING_TARGETS).build()
}

#[cfg(test)]
mod tests {
    use super::*;
    use tauri::plugin::Plugin;
    use tauri::test::MockRuntime;

    #[test]
    fn the_plugin_is_the_log_plugin() {
        let plugin = plugin::<MockRuntime>();
        assert_eq!(Plugin::<MockRuntime>::name(&plugin), "log");
    }
}
