//! The phone's chrome (glass buttons, menus, sheets) is SwiftUI drawn over the webview.
//! The web store owns every piece of state; this plugin renders what it is handed and
//! reports what the user asked for back as `intent` events.

mod font;
mod frontend;
mod frontend_asset;
mod ffi;
mod tauri_plugin_crowbar_chrome;

pub use tauri_plugin_crowbar_chrome::init;
