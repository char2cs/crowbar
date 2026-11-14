//! Registers the commands the Swift plugin handles with the Tauri permission system.

const COMMANDS: &[&str] = &[
    "set_chrome",
    "set_sidebar",
    "set_agents_settings",
    "set_hosts_settings",
    "set_host_picker",
    "set_files",
    "set_tabs",
    "set_model_picker",
    "set_plus_menu",
    "set_attach_sheet",
    "set_jump_to_latest",
    "confirm",
    "prompt",
    "register_listener",
    "remove_listener",
];

fn main() {
    tauri_plugin::Builder::new(COMMANDS).ios_path("../ios").build();
}
