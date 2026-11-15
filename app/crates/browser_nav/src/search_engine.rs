const QUERY_SLOT: &str = "%s";

/// The search engine the address bar falls back to for text that is not a host.
#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum SearchEngine {
    /// `DuckDuckGo`, the default.
    DuckDuckGo,
    /// Google.
    Google,
    /// Bing.
    Bing,
    /// Brave Search.
    Brave,
    /// A user-chosen `http(s)` URL containing `%s` where the query goes.
    Custom(String),
}

impl SearchEngine {
    /// The URL template, `%s` marking the query. A custom template that is not an `http(s)`
    /// URL with a `%s` falls back to `DuckDuckGo`.
    #[must_use]
    pub fn template(&self) -> &str {
        match self {
            Self::DuckDuckGo => "https://duckduckgo.com/?q=%s",
            Self::Google => "https://www.google.com/search?q=%s",
            Self::Bing => "https://www.bing.com/search?q=%s",
            Self::Brave => "https://search.brave.com/search?q=%s",
            Self::Custom(template) if is_usable(template.trim()) => template.trim(),
            Self::Custom(_) => Self::DuckDuckGo.template(),
        }
    }
}

fn is_usable(template: &str) -> bool {
    let web = template.starts_with("http://") || template.starts_with("https://");
    web && template.contains(QUERY_SLOT) && !template.contains(char::is_whitespace)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_usable_custom_template_is_kept_and_a_broken_one_falls_back() {
        let cases = [
            (
                "https://kagi.com/search?q=%s",
                "https://kagi.com/search?q=%s",
            ),
            (
                " https://kagi.com/search?q=%s ",
                "https://kagi.com/search?q=%s",
            ),
            ("https://kagi.com/search", "https://duckduckgo.com/?q=%s"),
            ("ftp://x/%s", "https://duckduckgo.com/?q=%s"),
            ("https://a b/%s", "https://duckduckgo.com/?q=%s"),
        ];
        for (custom, expected) in cases {
            assert_eq!(
                SearchEngine::Custom(custom.to_owned()).template(),
                expected,
                "{custom:?}"
            );
        }
    }

    #[test]
    fn every_builtin_engine_has_a_query_slot() {
        for engine in [
            SearchEngine::DuckDuckGo,
            SearchEngine::Google,
            SearchEngine::Bing,
            SearchEngine::Brave,
        ] {
            assert!(engine.template().contains("%s"), "{engine:?}");
        }
    }
}
