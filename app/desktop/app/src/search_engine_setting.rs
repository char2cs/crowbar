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

    const KAGI: &str = "https://kagi.com/search?q=%s";
    const DUCK: &str = "https://duckduckgo.com/?q=%s";

    struct Row {
        engine: Option<&'static str>,
        custom: Option<&'static str>,
        expected: &'static str,
    }

    const fn row(
        engine: Option<&'static str>,
        custom: Option<&'static str>,
        expected: &'static str,
    ) -> Row {
        Row {
            engine,
            custom,
            expected,
        }
    }

    const CASES: &[Row] = &[
        row(None, None, DUCK),
        row(Some("duckduckgo"), None, DUCK),
        row(Some("google"), None, "https://www.google.com/search?q=%s"),
        row(Some("custom"), Some(" https://kagi.com/search?q=%s "), KAGI),
        row(Some("custom"), Some("https://kagi.com/search"), DUCK),
        row(Some("custom"), None, DUCK),
        row(Some("unknown"), Some(KAGI), DUCK),
    ];

    #[test]
    fn the_stored_engine_picks_its_template_and_a_broken_custom_one_falls_back() {
        for case in CASES {
            let template = engine_from(case.engine, case.custom).template().to_owned();
            assert_eq!(
                template, case.expected,
                "{:?} {:?}",
                case.engine, case.custom
            );
        }
    }
}
