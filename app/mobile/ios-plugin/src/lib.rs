//! The phone's chrome (glass buttons, menus, sheets) is SwiftUI drawn over the webview.
//! The web store owns every piece of state; this plugin renders what it is handed and
//! reports what the user asked for back as `intent` events.

use tauri::plugin::{Builder, TauriPlugin};
use tauri::Runtime;

#[cfg(target_os = "ios")]
mod ios;

pub fn init<R: Runtime>() -> TauriPlugin<R> {
    #[allow(unused_mut)] // reason: only iOS adds the Swift plugin and its commands
    let mut builder = Builder::new("crowbar-chrome");
    #[cfg(target_os = "ios")]
    {
        builder = ios::attach(builder);
    }
    builder.build()
}
