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
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "wv-{}", self.0)
    }
}

#[cfg(test)]
#[path = "web_view_id_tests.rs"]
mod tests;
