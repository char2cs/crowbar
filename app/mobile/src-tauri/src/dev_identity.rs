//! Debug builds mark themselves (Dock icon, window title) so a developer — or an
//! agent — can tell a dev instance from production and from other worktrees' instances.
//! Compiled only with `debug_assertions`, so none of it reaches a release binary.

/// Badged dev artwork. Its ~10% transparent padding is load-bearing: AppKit draws
/// `-setApplicationIconImage:` edge-to-edge, so a tight crop looks oversized in the Dock.
#[cfg(target_os = "macos")]
const DEV_ICON_PNG: &[u8] = include_bytes!("../../icons-src/crowbar-dev.png");

/// Swaps the Dock icon for the dev artwork. Must run on `RunEvent::Ready`: tauri sets the
/// bundle icon itself on Ready in dev, overwriting anything set earlier in `setup()`.
/// Best-effort: every failure leaves the icon as it is.
#[cfg(target_os = "macos")]
pub fn apply_app_icon() {
    use objc2::msg_send;
    use objc2::runtime::{AnyClass, AnyObject};

    unsafe {
        let (Some(data_cls), Some(image_cls), Some(app_cls)) = (
            AnyClass::get(c"NSData"),
            AnyClass::get(c"NSImage"),
            AnyClass::get(c"NSApplication"),
        ) else {
            return;
        };

        // +dataWithBytes:length: copies, so the borrow of DEV_ICON_PNG ends here.
        let data: *mut AnyObject = msg_send![
            data_cls,
            dataWithBytes: DEV_ICON_PNG.as_ptr().cast::<std::ffi::c_void>(),
            length: DEV_ICON_PNG.len(),
        ];
        if data.is_null() {
            return;
        }

        // Deliberately never released: NSApp holds the icon for the process lifetime.
        let image: *mut AnyObject = msg_send![image_cls, alloc];
        let image: *mut AnyObject = msg_send![image, initWithData: data];
        if image.is_null() {
            return;
        }

        let ns_app: *mut AnyObject = msg_send![app_cls, sharedApplication];
        if ns_app.is_null() {
            return;
        }

        let _: () = msg_send![ns_app, setApplicationIconImage: image];
    }
    log::info!("dev build: Dock icon set to badged dev artwork");
}

/// The title a debug build wears, or `None` for a blank label: `Crowbar — dev: `
/// would look like it names a worktree while naming none.
#[cfg(desktop)]
fn window_title(label: &str) -> Option<String> {
    let label = label.trim();
    (!label.is_empty()).then(|| format!("Crowbar — dev: {label}"))
}

/// Names the worktree (`CROWBAR_DEV_LABEL`, a branch exported by the Makefile) in the
/// main window's title, so several dev instances on one machine stay distinguishable.
/// `setup` is early enough: tauri never revisits config-declared windows' titles.
#[cfg(desktop)]
pub fn apply_window_title(app: &tauri::App) {
    use tauri::Manager;

    let Some(title) = std::env::var("CROWBAR_DEV_LABEL")
        .ok()
        .and_then(|label| window_title(&label))
    else {
        return;
    };

    let Some(window) = app.get_window("main") else {
        return;
    };

    match window.set_title(&title) {
        Ok(()) => log::info!("dev build: window titled {title:?}"),
        Err(e) => log::warn!("dev build: could not set window title: {e}"),
    }
}

#[cfg(all(test, desktop))]
mod tests {
    use super::window_title;

    #[test]
    fn a_branch_name_becomes_the_title() {
        assert_eq!(
            window_title("feature/crowbar-skill").as_deref(),
            Some("Crowbar — dev: feature/crowbar-skill")
        );
    }

    /// `CROWBAR_DEV_LABEL` is a `$(shell git ...)`: a git failure arrives as "" and a
    /// stray newline as whitespace. Both must fall back to the configured title.
    #[test]
    fn a_label_git_could_not_produce_falls_back_to_the_plain_title() {
        assert_eq!(window_title(""), None);
        assert_eq!(window_title("   \n"), None);
    }
}
