//! The native "Toggle Console" Window-menu item. macOS consumes Cmd+` (window
//! cycle) before the webview sees it, so a menu accelerator is the only way the
//! default chord can fire. The keymap store stays the single owner of the chord:
//! the web pushes the effective one here and this only mirrors it.

#[cfg(target_os = "macos")]
use tauri::Emitter;
use tauri::Manager;

pub const WINDOW_MENU_ID: &str = "window_menu";
pub const CONSOLE_MENU_ID: &str = "toggle_console";
const TOGGLE_EVENT: &str = "console:toggle";

/// Translate a normalized keymap chord (`mod+shift+\``) into a menu accelerator
/// (`CmdOrCtrl+Shift+\``). `None` for anything a menu cannot express.
pub fn accelerator_for(chord: &str) -> Option<String> {
    let mut parts: Vec<&str> = Vec::new();
    let (mut modifier, mut shift, mut alt) = (false, false, false);
    let mut key: Option<String> = None;
    for part in chord
        .trim()
        .split('+')
        .map(str::trim)
        .filter(|p| !p.is_empty())
    {
        match part.to_lowercase().as_str() {
            "mod" | "cmd" | "ctrl" | "control" | "meta" => modifier = true,
            "shift" => shift = true,
            "alt" | "option" | "opt" => alt = true,
            other => key = Some(menu_key(other)?),
        }
    }
    let key = key?;
    if modifier {
        parts.push("CmdOrCtrl");
    }
    if shift {
        parts.push("Shift");
    }
    if alt {
        parts.push("Alt");
    }
    parts.push(&key);
    Some(parts.join("+"))
}

fn menu_key(key: &str) -> Option<String> {
    let named = match key {
        "arrowleft" => "Left",
        "arrowright" => "Right",
        "arrowup" => "Up",
        "arrowdown" => "Down",
        "enter" => "Enter",
        "escape" => "Escape",
        "tab" => "Tab",
        "space" => "Space",
        "backspace" => "Backspace",
        "delete" => "Delete",
        _ => "",
    };
    if !named.is_empty() {
        return Some(named.to_string());
    }
    let mut chars = key.chars();
    match (chars.next(), chars.next()) {
        (Some(c), None) if c != '+' => Some(c.to_uppercase().to_string()),
        _ => None,
    }
}

#[cfg(target_os = "macos")]
pub fn menu_item(app: &tauri::AppHandle) -> tauri::Result<tauri::menu::MenuItem<tauri::Wry>> {
    tauri::menu::MenuItemBuilder::new("Toggle Console")
        .id(CONSOLE_MENU_ID)
        .build(app)
}

/// Emit to the focused window only: each window owns its own console.
#[cfg(target_os = "macos")]
pub fn handle_menu_event(app: &tauri::AppHandle, id: &str) {
    if id != CONSOLE_MENU_ID {
        return;
    }
    let Some(window) = app.get_focused_window() else {
        log::debug!("Toggle Console: no focused window");
        return;
    };
    if let Err(e) = app.emit_to(window.label(), TOGGLE_EVENT, ()) {
        log::error!("Toggle Console menu item failed: {e}");
    }
}

/// Set (or clear, on `None`) the menu item's accelerator to the web's effective
/// chord. A chord the menu cannot express clears it rather than leaving a stale one.
#[tauri::command]
pub fn set_console_menu_chord(app: tauri::AppHandle, chord: Option<String>) -> Result<(), String> {
    #[cfg(target_os = "macos")]
    {
        let accelerator = chord.as_deref().and_then(accelerator_for);
        let item = app
            .menu()
            .and_then(|menu| menu.get(WINDOW_MENU_ID))
            .and_then(|kind| kind.as_submenu().and_then(|sub| sub.get(CONSOLE_MENU_ID)))
            .and_then(|kind| kind.as_menuitem().cloned())
            .ok_or_else(|| "console menu item is not built".to_string())?;
        item.set_accelerator(accelerator)
            .map_err(|e| e.to_string())?;
    }
    #[cfg(not(target_os = "macos"))]
    let _ = (app, chord);
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::accelerator_for;

    #[test]
    fn maps_the_default_chord() {
        assert_eq!(accelerator_for("mod+`").as_deref(), Some("CmdOrCtrl+`"));
    }

    #[test]
    fn maps_modifier_order_letters_and_named_keys() {
        assert_eq!(
            accelerator_for("mod+shift+alt+j").as_deref(),
            Some("CmdOrCtrl+Shift+Alt+J")
        );
        assert_eq!(
            accelerator_for("alt+arrowleft").as_deref(),
            Some("Alt+Left")
        );
    }

    #[test]
    fn refuses_what_a_menu_cannot_express() {
        assert_eq!(accelerator_for(""), None);
        assert_eq!(accelerator_for("mod+"), None);
        assert_eq!(accelerator_for("mod+f13"), None);
    }
}
