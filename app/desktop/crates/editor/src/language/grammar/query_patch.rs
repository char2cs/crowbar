use crate::Error;

/// Anchored rewrites of one language's highlight query.
#[derive(Clone, Copy, Debug)]
pub struct QueryPatch {
    /// The language whose query is rewritten, as gpui-kit's registry names it.
    pub language: &'static str,
    /// Each text to find, exactly once, and what replaces it.
    pub hunks: &'static [(&'static str, &'static str)],
}

impl QueryPatch {
    /// `query` with every hunk applied; a query that already has them is returned as it is.
    ///
    /// # Errors
    ///
    /// [`Error::QueryAnchor`] when a hunk's text is missing or not unique, which means the query
    /// changed under it.
    pub fn apply(
        &self,
        query: &str,
    ) -> Result<String, Error> {
        if self.is_applied(query) {
            return Ok(query.to_owned());
        }
        self.hunks
            .iter()
            .try_fold(query.to_owned(), |query, hunk| self.applied(&query, *hunk))
    }

    fn applied(
        &self,
        query: &str,
        (anchor, replacement): (&'static str, &'static str),
    ) -> Result<String, Error> {
        let error = Error::QueryAnchor {
            language: self.language,
            anchor,
        };
        let unique = query.matches(anchor).count() == 1;
        unique
            .then(|| query.replacen(anchor, replacement, 1))
            .ok_or(error)
    }

    fn is_applied(
        &self,
        query: &str,
    ) -> bool {
        self.hunks
            .iter()
            .all(|(anchor, replacement)| !query.contains(anchor) && query.contains(replacement))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const PATCH: QueryPatch = QueryPatch {
        language: "test",
        hunks: &[("(a) @x", "(a) @y")],
    };

    #[test]
    fn each_hunk_replaces_its_one_anchor_and_a_missing_or_repeated_one_fails() {
        let cases = [
            ("(b) @z\n(a) @x\n", Some("(b) @z\n(a) @y\n")),
            ("(a) @y", Some("(a) @y")),
            ("(b) @z", None),
            ("(a) @x (a) @x", None),
        ];
        for (query, expected) in cases {
            let applied = PATCH.apply(query);
            assert_eq!(applied.as_deref().ok(), expected, "{query}");
            let failed = matches!(applied, Err(Error::QueryAnchor { .. }));
            assert_eq!(failed, expected.is_none(), "{query}");
        }
    }
}
