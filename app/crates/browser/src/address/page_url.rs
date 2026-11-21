use url::{Host, Url};

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

    /// Whether the page is served by the computer that opens it: localhost and its subdomains,
    /// 127.0.0.0/8, 0.0.0.0, `[::1]`, `[::]` and IPv4-mapped loopback addresses.
    #[must_use]
    pub fn is_loopback(&self) -> bool {
        let Ok(url) = Url::parse(&self.0) else {
            return false;
        };
        match url.host() {
            Some(Host::Domain(name)) => is_local_name(name),
            Some(Host::Ipv4(address)) => address.is_loopback() || address.is_unspecified(),
            Some(Host::Ipv6(address)) => {
                let mapped = address.to_ipv4_mapped();
                address.is_loopback()
                    || address.is_unspecified()
                    || mapped.is_some_and(|address| address.is_loopback())
            },
            None => false,
        }
    }

    /// The URL text.
    #[must_use]
    pub fn as_str(&self) -> &str {
        &self.0
    }
}

fn is_local_name(name: &str) -> bool {
    let name = name.trim_end_matches('.').to_ascii_lowercase();
    name == "localhost" || name.ends_with(".localhost")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn loopback_pages_are_told_from_pages_of_other_computers() {
        const CASES: &[(&str, bool)] = &[
            ("http://localhost:3000/a", true),
            ("http://app.localhost/", true),
            ("http://127.0.0.2:8000", true),
            ("http://0.0.0.0:80", true),
            ("http://[::1]:5173", true),
            ("http://[::]/", true),
            ("http://[::ffff:127.0.0.1]/", true),
            ("https://example.com", false),
            ("http://10.0.0.1", false),
            ("http://localhost.example.com", false),
        ];
        for (text, loopback) in CASES {
            let url = PageUrl::parse(text).unwrap();
            assert_eq!(url.is_loopback(), *loopback, "{text}");
        }
    }

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
