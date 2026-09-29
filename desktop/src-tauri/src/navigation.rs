//! The webview only ever shows Crowbar's own UI; any other URL is handed to the
//! OS default browser instead of replacing the app.
use tauri::Url;

#[derive(Debug, PartialEq, Eq)]
pub enum Navigation {
    Allow,
    OpenExternally,
    Block,
}

const APP_SCHEMES: [&str; 7] = ["tauri", "crowbar", "asset", "ipc", "about", "blob", "data"];
const EXTERNAL_SCHEMES: [&str; 4] = ["http", "https", "mailto", "tel"];

pub fn classify(url: &Url, dev_url: Option<&Url>) -> Navigation {
    if APP_SCHEMES.contains(&url.scheme()) {
        return Navigation::Allow;
    }
    if url.host_str() == Some("tauri.localhost") {
        return Navigation::Allow;
    }
    if dev_url.is_some_and(|dev| dev.origin() == url.origin()) {
        return Navigation::Allow;
    }
    if EXTERNAL_SCHEMES.contains(&url.scheme()) {
        return Navigation::OpenExternally;
    }
    Navigation::Block
}

/// Decides one webview navigation; returns whether the webview may proceed.
pub fn guard(url: &Url, dev_url: Option<&Url>) -> bool {
    match classify(url, dev_url) {
        Navigation::Allow => true,
        Navigation::OpenExternally => {
            if let Err(e) = tauri_plugin_opener::open_url(url.as_str(), None::<&str>) {
                log::warn!("navigation: could not open {url} externally: {e}");
            }
            false
        }
        Navigation::Block => false,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn url(s: &str) -> Url {
        Url::parse(s).unwrap()
    }

    #[test]
    fn app_origins_stay_in_the_webview() {
        let dev = url("http://localhost:5173");
        assert_eq!(
            classify(&url("tauri://localhost/index.html"), None),
            Navigation::Allow
        );
        assert_eq!(
            classify(&url("http://tauri.localhost/"), None),
            Navigation::Allow
        );
        assert_eq!(
            classify(&url("crowbar://localhost/v0/x"), None),
            Navigation::Allow
        );
        assert_eq!(
            classify(&url("http://localhost:5173/#/a"), Some(&dev)),
            Navigation::Allow
        );
    }

    #[test]
    fn web_links_leave_the_app() {
        let dev = url("http://localhost:5173");
        assert_eq!(
            classify(&url("https://example.com/a"), Some(&dev)),
            Navigation::OpenExternally
        );
        assert_eq!(
            classify(&url("http://localhost:3000/"), Some(&dev)),
            Navigation::OpenExternally
        );
        assert_eq!(
            classify(&url("mailto:a@b.c"), None),
            Navigation::OpenExternally
        );
    }

    #[test]
    fn unknown_schemes_are_blocked() {
        assert_eq!(
            classify(&url("file:///etc/passwd"), None),
            Navigation::Block
        );
        assert_eq!(
            classify(&url("javascript:alert(1)"), None),
            Navigation::Block
        );
    }
}
