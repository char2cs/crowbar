//! The phone's chrome (glass buttons, menus, sheets) is SwiftUI drawn over the webview.
//! The web store owns every piece of state; this plugin renders what it is handed and
//! reports what the user asked for back as `intent` events.

mod ffi;
mod font;
mod frontend;
mod frontend_asset;
mod ios_plugin;

pub use ios_plugin::init;
