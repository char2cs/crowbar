//! The Tauri app of the phone: plugins, managed state and the command table.

mod api_proxy;
mod host_commands;
mod host_error;
mod host_route;
mod host_tokens;
mod hosts;
mod init_scripts;
mod logging;
mod navigation;
mod src_tauri;
#[cfg(test)]
mod test_support;
mod tls_pin;
mod token_error;
mod ws_bridge;

pub use src_tauri::run;
