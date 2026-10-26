const COMMANDS: &[&str] = &["set_chrome", "set_sidebar", "set_agents_settings", "set_hosts_settings", "set_files", "set_tabs", "set_context", "set_model_picker", "confirm", "prompt", "register_listener", "remove_listener"];

fn main() {
    tauri_plugin::Builder::new(COMMANDS).ios_path("ios").build();
}
