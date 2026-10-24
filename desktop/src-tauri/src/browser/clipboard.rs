//! Plain-text copy through the OS pasteboard. A pick finishes by a click inside
//! a native child webview, so the app document has no user activation and
//! `navigator.clipboard` is refused.

#[cfg(target_os = "macos")]
fn write_text(text: &str) -> Result<(), String> {
    use objc2_app_kit::{NSPasteboard, NSPasteboardTypeString};
    use objc2_foundation::NSString;

    let pasteboard = NSPasteboard::generalPasteboard();
    pasteboard.clearContents();
    // SAFETY: NSPasteboardTypeString is a valid, immutable AppKit constant.
    let ok =
        pasteboard.setString_forType(&NSString::from_str(text), unsafe { NSPasteboardTypeString });
    ok.then_some(())
        .ok_or_else(|| "the pasteboard rejected the text".to_string())
}

#[cfg(not(target_os = "macos"))]
fn write_text(_text: &str) -> Result<(), String> {
    Err("copying from a browser tab is only supported on macOS".into())
}

#[tauri::command]
pub fn browser_copy_text(text: String) -> Result<(), String> {
    write_text(&text)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[cfg(target_os = "macos")]
    #[test]
    fn text_lands_on_the_general_pasteboard() {
        use objc2_app_kit::{NSPasteboard, NSPasteboardTypeString};
        let read = || {
            NSPasteboard::generalPasteboard()
                .stringForType(unsafe { NSPasteboardTypeString })
                .map(|s| s.to_string())
        };
        let previous = read();
        let text = "crowbar pointer \u{2316} test";
        browser_copy_text(text.into()).unwrap();
        assert_eq!(read().as_deref(), Some(text));
        if let Some(previous) = previous {
            browser_copy_text(previous).unwrap();
        }
    }

    #[cfg(not(target_os = "macos"))]
    #[test]
    fn other_platforms_report_the_limit() {
        assert!(browser_copy_text("x".into()).unwrap_err().contains("macOS"));
    }
}
