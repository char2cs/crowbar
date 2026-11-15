use std::fmt;

/// Identifies one web view of a [`WebViewHost`](crate::WebViewHost) for its whole life.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub struct WebViewId(pub(crate) u64);

impl WebViewId {
    /// Parses the text form produced by [`Display`](fmt::Display).
    #[must_use]
    pub fn parse(text: &str) -> Option<Self> {
        text.strip_prefix("wv-")?.parse().ok().map(Self)
    }
}

impl fmt::Display for WebViewId {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        write!(f, "wv-{}", self.0)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn text_form_round_trips() {
        let id = WebViewId(42);
        assert_eq!(id.to_string(), "wv-42", "display");
        assert_eq!(WebViewId::parse("wv-42"), Some(id), "parse");
        assert_eq!(WebViewId::parse("42"), None, "missing prefix");
        assert_eq!(WebViewId::parse("wv-x"), None, "not a number");
    }
}
