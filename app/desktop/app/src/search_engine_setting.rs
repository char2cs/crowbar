use browser_nav::SearchEngine;

pub(crate) const ENGINE_KEY: &str = "browserSearchEngine";
pub(crate) const CUSTOM_URL_KEY: &str = "browserCustomSearchUrl";

pub(crate) fn engine_from(
    engine: Option<&str>,
    custom_url: Option<&str>,
) -> SearchEngine {
    match engine {
        Some("google") => SearchEngine::Google,
        Some("bing") => SearchEngine::Bing,
        Some("brave") => SearchEngine::Brave,
        Some("custom") => SearchEngine::Custom(custom_url.unwrap_or_default().to_owned()),
        _ => SearchEngine::DuckDuckGo,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_stored_engine_picks_its_template_and_a_broken_custom_one_falls_back() {
        const KAGI: &str = "https://kagi.com/search?q=%s";
        const DUCK: &str = "https://duckduckgo.com/?q=%s";
        type Case = (Option<&'static str>, Option<&'static str>, &'static str);
        const CASES: &[Case] = &[
            (None, None, DUCK),
            (Some("duckduckgo"), None, DUCK),
            (Some("google"), None, "https://www.google.com/search?q=%s"),
            (Some("custom"), Some(" https://kagi.com/search?q=%s "), KAGI),
            (Some("custom"), Some("https://kagi.com/search"), DUCK),
            (Some("custom"), None, DUCK),
            (Some("unknown"), Some(KAGI), DUCK),
        ];
        for (engine, custom, expected) in CASES {
            assert_eq!(
                engine_from(*engine, *custom).template(),
                *expected,
                "{engine:?} {custom:?}"
            );
        }
    }
}
