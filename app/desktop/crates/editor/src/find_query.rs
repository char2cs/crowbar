pub(crate) mod find_option;
pub(crate) mod find_options;
pub(crate) mod word;

use std::ops::Range;

use regex::{Regex, RegexBuilder};

use crate::{Error, FindOption, FindOptions};

// SPEC: https://github.com/microsoft/vscode/blob/main/src/vs/editor/contrib/find/browser/findModel.ts (MATCHES_LIMIT)
const MATCHES_LIMIT: usize = 19_999;

/// A text to find the occurrences of, and the toggles that shape the search.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct FindQuery {
    text: String,
    options: FindOptions,
}

impl FindQuery {
    /// A query for `text` under `options`.
    #[must_use]
    pub fn new(
        text: &str,
        options: FindOptions,
    ) -> Self {
        Self {
            text: text.to_owned(),
            options,
        }
    }

    /// Every non-empty match in `haystack`, in order, up to a limit; none for an empty query.
    ///
    /// # Errors
    /// [`Error::Pattern`] when the text is too large to search for.
    pub fn matches(
        &self,
        haystack: &str,
    ) -> Result<Vec<Range<usize>>, Error> {
        if self.text.is_empty() {
            return Ok(Vec::new());
        }
        let pattern = self.pattern()?;
        let whole_word = self.options.has(FindOption::WholeWord);
        let found = pattern
            .find_iter(haystack)
            .map(|found| found.range())
            .filter(|range| !range.is_empty())
            .filter(|range| !whole_word || bounded_by_separators(haystack, range))
            .take(MATCHES_LIMIT)
            .collect();
        Ok(found)
    }

    fn pattern(&self) -> Result<Regex, Error> {
        RegexBuilder::new(&regex::escape(&self.text))
            .case_insensitive(!self.options.has(FindOption::MatchCase))
            .multi_line(true)
            .build()
            .map_err(|source| Error::Pattern {
                pattern: self.text.clone(),
                source,
            })
    }
}

fn bounded_by_separators(
    haystack: &str,
    range: &Range<usize>,
) -> bool {
    let text = haystack.get(range.clone()).unwrap_or("");
    let before = haystack
        .get(..range.start)
        .and_then(|head| head.chars().next_back());
    let after = haystack
        .get(range.end..)
        .and_then(|tail| tail.chars().next());
    let left = edge_is_boundary(before, text.chars().next());
    let right = edge_is_boundary(after, text.chars().next_back());
    left && right
}

fn edge_is_boundary(
    outside: Option<char>,
    inside: Option<char>,
) -> bool {
    let Some(outside) = outside else {
        return true;
    };
    word::is_separator(outside) || inside.is_some_and(word::is_separator)
}

#[cfg(test)]
mod tests {
    use super::*;

    type Toggles = &'static [FindOption];
    type Spans = &'static [(usize, usize)];

    const MATCHED: &str = "Price price priced\nprice";
    const MATCH_CASES: &[(&str, Toggles, Spans)] = &[
        ("price", &[], &[(0, 5), (6, 11), (12, 17), (19, 24)]),
        (
            "price",
            &[FindOption::MatchCase],
            &[(6, 11), (12, 17), (19, 24)],
        ),
        (
            "price",
            &[FindOption::WholeWord],
            &[(0, 5), (6, 11), (19, 24)],
        ),
        ("p.ice", &[], &[]),
        ("", &[], &[]),
    ];

    #[test]
    fn a_query_matches_as_its_toggles_say() {
        for (query, on, spans) in MATCH_CASES {
            let expected: Vec<Range<usize>> =
                spans.iter().map(|(start, end)| *start..*end).collect();
            let found = FindQuery::new(query, FindOptions::of(on)).matches(MATCHED);
            assert_eq!(found.ok(), Some(expected), "{query:?} {on:?}");
        }
    }
}
