//! Native browser tabs: one Tauri child webview per tab, owned by Tauri's own
//! webview manager and addressed by the label `browser-<id>`.
mod agent;
mod clipboard;
mod commands;
mod data;
mod events;
mod park;
#[cfg(target_os = "macos")]
mod webkit;

pub use agent::*;
pub use clipboard::*;
pub use commands::*;
pub use data::*;
pub use park::Parked;

use tauri::{AppHandle, Runtime, Url, Webview};

const LABEL_PREFIX: &str = "browser-";

fn valid_id(id: &str) -> bool {
    !id.is_empty()
        && id
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'_' || b == b'-')
}

pub fn label_for(id: &str) -> Result<String, String> {
    if !valid_id(id) {
        return Err(format!("invalid browser tab id {id:?}"));
    }
    Ok(format!("{LABEL_PREFIX}{id}"))
}

pub fn id_from_label(label: &str) -> Option<&str> {
    label.strip_prefix(LABEL_PREFIX).filter(|id| valid_id(id))
}

/// Browser webviews show arbitrary web pages, so the app navigation guard
/// (which sends web links to the OS browser) must not apply to them.
pub fn exempt_from_app_guard(label: &str) -> bool {
    id_from_label(label).is_some()
}

pub fn parse_url(raw: &str) -> Result<Url, String> {
    let url = Url::parse(raw).map_err(|e| format!("invalid url {raw:?}: {e}"))?;
    if is_allowed(&url) {
        return Ok(url);
    }
    Err(format!("unsupported url scheme {:?}", url.scheme()))
}

fn is_allowed(url: &Url) -> bool {
    matches!(url.scheme(), "http" | "https") || url.as_str() == "about:blank"
}

pub fn find<R: Runtime>(app: &AppHandle<R>, id: &str) -> Result<Webview<R>, String> {
    let label = label_for(id)?;
    use tauri::Manager;
    app.get_webview(&label)
        .ok_or_else(|| format!("no browser webview for {id:?}"))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn labels_follow_the_browser_scheme() {
        assert_eq!(label_for("tab_1-a").unwrap(), "browser-tab_1-a");
        assert_eq!(id_from_label("browser-tab_1-a"), Some("tab_1-a"));
        assert_eq!(id_from_label("main"), None);
        assert_eq!(id_from_label("browser-"), None);
    }

    #[test]
    fn ids_outside_the_safe_alphabet_are_rejected() {
        for bad in ["", "a b", "a/b", "a.b", "é", "a:b"] {
            assert!(label_for(bad).is_err(), "{bad:?}");
        }
    }

    #[test]
    fn only_web_urls_and_about_blank_are_navigable() {
        assert!(parse_url("https://example.com/a?b=1").is_ok());
        assert!(parse_url("http://localhost:3000").is_ok());
        assert!(parse_url("about:blank").is_ok());
        for bad in [
            "file:///etc/passwd",
            "javascript:alert(1)",
            "crowbar://localhost/v0/x",
            "tauri://localhost",
            "data:text/html,hi",
            "about:srcdoc",
            "not a url",
        ] {
            assert!(parse_url(bad).is_err(), "{bad}");
        }
    }

    #[test]
    fn guard_exemption_applies_to_browser_labels_only() {
        assert!(exempt_from_app_guard("browser-abc"));
        assert!(!exempt_from_app_guard("main"));
        assert!(!exempt_from_app_guard("w2"));
        assert!(!exempt_from_app_guard("browser-"));
        assert!(!exempt_from_app_guard("browser-a b"));
    }
}
