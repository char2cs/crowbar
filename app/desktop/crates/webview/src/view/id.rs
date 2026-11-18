use std::fmt;

/// Identifies one web view of a [`Host`](crate::Host) for its whole life.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub struct Id(pub(crate) u64);

impl Id {
    /// Parses the text form produced by [`Display`](fmt::Display).
    #[must_use]
    pub fn parse(text: &str) -> Option<Self> {
        text.strip_prefix("wv-")?.parse().ok().map(Self)
    }
}

impl fmt::Display for Id {
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
        let id = Id(42);
        assert_eq!(id.to_string(), "wv-42", "display");
        assert_eq!(Id::parse("wv-42"), Some(id), "parse");
        assert_eq!(Id::parse("42"), None, "missing prefix");
        assert_eq!(Id::parse("wv-x"), None, "not a number");
    }
}
