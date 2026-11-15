use url::Url;

const APP_SCHEMES: &[&str] = &["crowbar", "about"];
const EXTERNAL_SCHEMES: &[&str] = &["http", "https", "mailto", "tel"];

/// What to do with a link a page asked to follow.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum LinkTarget {
    /// The link is the app's own; the web view may load it.
    InApp,
    /// The link leaves the app and goes to the system browser.
    SystemBrowser,
    /// The link must not be followed at all.
    Refuse,
}

/// Decides where `link` goes. `dev_origin` is the dev server of a development build, which counts as the
/// app's own origin. `data:`, `blob:`, `file:` and `javascript:` are never followed: a document of
/// either of the first two has an opaque origin and must not replace the app.
#[must_use]
pub fn classify(
    link: &str,
    dev_origin: Option<&str>,
) -> LinkTarget {
    let Ok(url) = Url::parse(link) else {
        return LinkTarget::Refuse;
    };
    let is_dev = dev_origin
        .and_then(|origin| Url::parse(origin).ok())
        .is_some_and(|dev| dev.origin() == url.origin() && dev.origin().is_tuple());
    if APP_SCHEMES.contains(&url.scheme()) || is_dev {
        return LinkTarget::InApp;
    }
    if EXTERNAL_SCHEMES.contains(&url.scheme()) {
        return LinkTarget::SystemBrowser;
    }
    LinkTarget::Refuse
}

#[cfg(test)]
mod tests {
    use super::*;

    const DEV: &str = "http://localhost:5173";

    const CASES: &[(&str, LinkTarget)] = &[
        ("crowbar://core/panes/chat/index.html", LinkTarget::InApp),
        ("about:blank", LinkTarget::InApp),
        ("about:srcdoc", LinkTarget::InApp),
        ("http://localhost:5173/#/a", LinkTarget::InApp),
        ("https://example.com/a", LinkTarget::SystemBrowser),
        ("http://localhost:3000/", LinkTarget::SystemBrowser),
        ("mailto:a@b.c", LinkTarget::SystemBrowser),
        ("tel:+4912345", LinkTarget::SystemBrowser),
        (
            "data:text/html,<script>alert(1)</script>",
            LinkTarget::Refuse,
        ),
        ("blob:crowbar://core/0b1c6a52", LinkTarget::Refuse),
        ("file:///etc/passwd", LinkTarget::Refuse),
        ("javascript:alert(1)", LinkTarget::Refuse),
        ("-oProxyCommand=evil", LinkTarget::Refuse),
        ("", LinkTarget::Refuse),
    ];

    #[test]
    fn links_go_where_the_policy_says() {
        for (link, expected) in CASES {
            assert_eq!(classify(link, Some(DEV)), *expected, "{link}");
        }
    }

    #[test]
    fn the_dev_origin_is_only_the_app_when_one_is_given() {
        assert_eq!(
            classify("http://localhost:5173/", None),
            LinkTarget::SystemBrowser
        );
    }
}
