use url::Url;

use crate::error::Error;

const BLANK: &str = "about:blank";

/// A URL a browser pane may load: `http`, `https` or `about:blank`. `file:`, `javascript:`,
/// `data:` and the shell's own `crowbar:` scheme are never loadable in a browser pane.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PageUrl(String);

impl PageUrl {
    /// Whether `text` is the empty page every browser view starts on, which no request waits for.
    #[must_use]
    pub fn is_blank(text: &str) -> bool {
        text == BLANK
    }

    /// Parses `text` and checks its scheme.
    ///
    /// # Errors
    /// [`Error::Invalid`] when `text` is not a URL, [`Error::Scheme`] for a refused scheme.
    pub fn parse(text: &str) -> Result<Self, Error> {
        if text == BLANK {
            return Ok(Self(BLANK.to_owned()));
        }
        let url = Url::parse(text).map_err(|source| Error::Invalid {
            text: text.to_owned(),
            source,
        })?;
        match url.scheme() {
            "http" | "https" => Ok(Self(url.into())),
            other => Err(Error::Scheme(other.to_owned())),
        }
    }

    /// Whether a browser pane may navigate to `text`.
    #[must_use]
    pub fn is_loadable(text: &str) -> bool {
        Self::parse(text).is_ok()
    }

    /// The URL text.
    #[must_use]
    pub fn as_str(&self) -> &str {
        &self.0
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_web_urls_and_the_blank_page_load() {
        let allowed = [
            "https://example.com/a?b=1",
            "http://localhost:3000",
            "about:blank",
        ];
        let refused = [
            "file:///etc/passwd",
            "javascript:alert(1)",
            "crowbar://core/panes/core/chat/index.html",
            "data:text/html,hi",
            "about:srcdoc",
            "blob:https://example.com/x",
            "not a url",
            "",
        ];
        for text in allowed {
            assert!(PageUrl::is_loadable(text), "{text}");
        }
        for text in refused {
            assert!(!PageUrl::is_loadable(text), "{text}");
        }
    }

    #[test]
    fn a_refused_scheme_is_named() {
        assert_eq!(
            PageUrl::parse("file:///x"),
            Err(Error::Scheme("file".to_owned()))
        );
    }
}
