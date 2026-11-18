//! Switches of a terminal search.

/// Match case, whole word and regular expression.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct SearchOptions(u8);

impl SearchOptions {
    /// Case-insensitive plain-text search.
    pub const NONE: Self = Self(0);
    /// Upper and lower case differ.
    pub const CASE_SENSITIVE: Self = Self(1);
    /// Matches must be whole words.
    pub const WHOLE_WORD: Self = Self(1 << 1);
    /// The text is a regular expression.
    pub const REGEX: Self = Self(1 << 2);

    /// True when every switch of `other` is on.
    pub fn contains(
        self,
        other: Self,
    ) -> bool {
        self.0 & other.0 == other.0
    }

    /// The union of both sets.
    #[must_use]
    pub fn with(
        self,
        other: Self,
    ) -> Self {
        Self(self.0 | other.0)
    }

    /// The set with the switches of `other` flipped.
    #[must_use]
    pub fn toggled(
        self,
        other: Self,
    ) -> Self {
        Self(self.0 ^ other.0)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn switches_combine_and_flip() {
        let options = SearchOptions::NONE.with(SearchOptions::REGEX);
        assert!(options.contains(SearchOptions::REGEX));
        assert!(!options.contains(SearchOptions::WHOLE_WORD));
        assert_eq!(options.toggled(SearchOptions::REGEX), SearchOptions::NONE);
    }
}
