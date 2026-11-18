use browser::{BrowserUrl, SearchEngine, resolve_address};
use protocol::PaneState;
use serde_json::json;

pub(crate) const HOMEPAGE_KEY: &str = "browserHomepage";

pub(crate) fn homepage_state(
    stored: Option<&str>,
    engine: &SearchEngine,
) -> PaneState {
    let url = stored
        .and_then(|typed| resolve_address(typed, engine))
        .and_then(|address| BrowserUrl::parse(&address).ok());
    match url {
        Some(url) => PaneState::new(json!({ "url": url.as_str() })).unwrap_or_default(),
        None => PaneState::default(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const KAGI: &str = "https://kagi.com/search?q=%s";

    type Case = (
        Option<&'static str>,
        Option<&'static str>,
        Option<&'static str>,
    );

    const CASES: [Case; 7] = [
        (None, None, None),
        (Some(""), None, None),
        (Some("   "), None, None),
        (Some("example.com"), None, Some("https://example.com/")),
        (Some("localhost:3000"), None, Some("http://localhost:3000/")),
        (
            Some("rust"),
            Some(KAGI),
            Some("https://kagi.com/search?q=rust"),
        ),
        (
            Some("rust"),
            Some("kagi"),
            Some("https://duckduckgo.com/?q=rust"),
        ),
    ];

    fn engine_of(custom: Option<&str>) -> SearchEngine {
        custom.map_or(SearchEngine::DuckDuckGo, |url| {
            SearchEngine::Custom(url.to_owned())
        })
    }

    #[test]
    fn the_homepage_becomes_the_first_address_and_blank_or_unloadable_ones_leave_the_bar_blank() {
        for (stored, custom, expected) in CASES {
            let engine = engine_of(custom);
            let state = homepage_state(stored, &engine);
            let url = state.as_value().get("url").and_then(|v| v.as_str());
            assert_eq!(url, expected, "{stored:?} {custom:?}");
        }
    }
}
