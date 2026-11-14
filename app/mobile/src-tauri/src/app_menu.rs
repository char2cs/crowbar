//! The macOS application menu. It omits the native accelerators the webview needs:
//! Undo/Redo and Select All (AppKit would target WKWebView's own undo and Monaco's
//! partial input buffer, not Monaco's model) and Close (Cmd+W is the in-app close-tab).

use tauri::menu::{AboutMetadata, Menu, MenuBuilder, MenuEvent, MenuItemBuilder, SubmenuBuilder};
use tauri::{AppHandle, Manager};

use crate::console_menu;

/// Shared by the builder and the event handler so the two cannot drift apart silently.
const NEW_WINDOW_MENU_ID: &str = "new_window";
const CLOSE_WINDOW_MENU_ID: &str = "close_window";

/// Accelerators here must not collide with the frontend keymap
/// (app/panes/src/features/keymaps/registry.ts): AppKit consumes a menu accelerator before
/// WKWebView sees the key, making the in-app binding unreachable and unrebindable.
pub fn build(app: &AppHandle) -> tauri::Result<Menu<tauri::Wry>> {
    let app_menu = SubmenuBuilder::new(app, "Crowbar")
        .about(Some(AboutMetadata::default()))
        .separator()
        .services()
        .separator()
        .hide()
        .hide_others()
        .show_all()
        .separator()
        .quit()
        .build()?;

    // Cut/Copy/Paste stay: WebKit fires them as DOM events Monaco intercepts.
    let edit_menu = SubmenuBuilder::new(app, "Edit")
        .cut()
        .copy()
        .paste()
        .build()?;

    // Built by hand for free chords: Cmd+N / Cmd+Shift+N are in-app actions, and with
    // Cmd+W rebound to close-tab, Cmd+Shift+W is the only keyboard way to close a window.
    let new_window = MenuItemBuilder::new("New Window")
        .id(NEW_WINDOW_MENU_ID)
        .accelerator("CmdOrCtrl+Alt+N")
        .build(app)?;
    let close_window = MenuItemBuilder::new("Close Window")
        .id(CLOSE_WINDOW_MENU_ID)
        .accelerator("CmdOrCtrl+Shift+W")
        .build(app)?;
    // Accelerator is set at runtime from the keymap (console_menu::set_console_menu_chord).
    let toggle_console = console_menu::menu_item(app)?;
    let window_menu = SubmenuBuilder::with_id(app, console_menu::WINDOW_MENU_ID, "Window")
        .item(&new_window)
        .item(&toggle_console)
        .item(&close_window)
        .separator()
        .minimize()
        .build()?;

    MenuBuilder::new(app)
        .items(&[&app_menu, &edit_menu, &window_menu])
        .build()
}

pub fn handle_event(app: &AppHandle, event: MenuEvent) {
    console_menu::handle_menu_event(app, event.id().as_ref());
    if event.id() == NEW_WINDOW_MENU_ID {
        let app = app.clone();
        // The command is async and menu events are sync: spawn so the menu
        // handler returns immediately rather than blocking the event loop.
        tauri::async_runtime::spawn(async move {
            if let Err(e) = crate::windows::open_window(app, "/".to_string()).await {
                log::error!("New Window menu item failed: {e}");
            }
        });
    } else if event.id() == CLOSE_WINDOW_MENU_ID {
        // `close()` requests a close, so it takes the same CloseRequested path as the
        // traffic light: transports retired, daemon stopped only after the last window.
        match app.get_focused_window() {
            Some(window) => {
                if let Err(e) = window.close() {
                    log::error!("Close Window menu item failed: {e}");
                }
            }
            // e.g. every window minimised while the app is frontmost; AppKit's own
            // Close would be disabled here, this item is not.
            None => log::debug!("Close Window: no focused window to close"),
        }
    }
}
