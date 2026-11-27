use tauri::plugin::{Builder, TauriPlugin};
use tauri::{Manager, Runtime, Url};

#[derive(Debug, PartialEq, Eq)]
pub(crate) enum Navigation {
    Allow,
    OpenExternally,
    Block,
}

// PLATFORM: ios webkit has no `data:`/`blob:` here: a document of either has an opaque origin and must never
// PLATFORM: ios webkit never routes their downloads through this check, so replacing the app is moot.
const APP_SCHEMES: [&str; 5] = ["tauri", "crowbar", "asset", "ipc", "about"];
const EXTERNAL_SCHEMES: [&str; 4] = ["http", "https", "mailto", "tel"];
const APP_HOST: &str = "tauri.localhost";

pub(crate) fn classify(
    url: &Url,
    dev_url: Option<&Url>,
) -> Navigation {
    if APP_SCHEMES.contains(&url.scheme()) || url.host_str() == Some(APP_HOST) {
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

fn open_externally(url: &Url) {
    if let Err(error) = tauri_plugin_opener::open_url(url.as_str(), None::<&str>) {
        log::warn!("navigation: could not open {url} externally: {error}");
    }
}

fn guard(
    url: &Url,
    dev_url: Option<&Url>,
) -> bool {
    match classify(url, dev_url) {
        Navigation::Allow => true,
        Navigation::OpenExternally => {
            open_externally(url);
            false
        },
        Navigation::Block => false,
    }
}

pub(crate) fn plugin<R: Runtime>() -> TauriPlugin<R> {
    Builder::<R, ()>::new("navigation-guard")
        .on_navigation(|webview, url| {
            let dev_url = webview.app_handle().config().build.dev_url.clone();
            guard(url, dev_url.as_ref())
        })
        .build()
}

#[cfg(test)]
mod tests {
    use super::*;
    use tauri::plugin::Plugin;
    use tauri::test::MockRuntime;

    const DEV: &str = "http://localhost:5173";

    #[test]
    fn classify_sorts_every_origin_into_stay_leave_or_block() {
        const CASES: &[(&str, bool, Navigation)] = &[
            ("tauri://localhost/index.html", false, Navigation::Allow),
            ("http://tauri.localhost/", false, Navigation::Allow),
            ("crowbar://localhost/v0/x", false, Navigation::Allow),
            ("http://localhost:5173/#/a", true, Navigation::Allow),
            ("about:srcdoc", false, Navigation::Allow),
            ("about:blank", false, Navigation::Allow),
            ("https://example.com/a", true, Navigation::OpenExternally),
            ("http://localhost:3000/", true, Navigation::OpenExternally),
            ("mailto:a@b.c", false, Navigation::OpenExternally),
            (
                "data:text/html,<script>alert(1)</script>",
                false,
                Navigation::Block,
            ),
            (
                "blob:tauri://localhost/0b1c6a52-4f0e-4f43-9d8e-1f2a3b4c5d6e",
                false,
                Navigation::Block,
            ),
            ("file:///etc/passwd", false, Navigation::Block),
            ("javascript:alert(1)", false, Navigation::Block),
        ];
        let dev = Url::parse(DEV).unwrap();
        for (target, with_dev, expected) in CASES {
            let url = Url::parse(target).unwrap();
            let actual = classify(&url, with_dev.then_some(&dev));
            assert_eq!(&actual, expected, "{target}");
        }
    }

    #[test]
    fn guard_lets_the_app_stay_and_refuses_everything_else() {
        const CASES: &[(&str, bool)] = &[
            ("crowbar://localhost/v0/x", true),
            ("file:///etc/passwd", false),
            ("javascript:alert(1)", false),
        ];
        for (target, stays) in CASES {
            let url = Url::parse(target).unwrap();
            assert_eq!(guard(&url, None), *stays, "{target}");
        }
    }

    #[test]
    fn the_plugin_is_named_for_its_capability() {
        let plugin = plugin::<MockRuntime>();
        assert_eq!(Plugin::<MockRuntime>::name(&plugin), "navigation-guard");
    }
}
