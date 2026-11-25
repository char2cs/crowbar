mod case_preserved;
pub(crate) mod find_option;
pub(crate) mod find_options;
mod replace_template;
pub(crate) mod word;

use std::ops::Range;

use regex::{Regex, RegexBuilder};

use crate::{Error, FindOption, FindOptions};

use self::case_preserved::case_preserved;
use self::replace_template::rust_template;

// SPEC: https://github.com/microsoft/vscode/blob/main/src/vs/editor/contrib/find/browser/findModel.ts (MATCHES_LIMIT)
const MATCHES_LIMIT: usize = 19_999;

/// What the find widget looks for: the typed text and the toggles that shape it.
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

    /// The text as typed.
    #[must_use]
    pub fn text(&self) -> &str {
        &self.text
    }

    /// The toggles in force.
    #[must_use]
    pub fn options(&self) -> &FindOptions {
        &self.options
    }

    /// Every non-empty match in `haystack`, in order, up to Monaco's limit; none for an empty
    /// query.
    ///
    /// # Errors
    /// [`Error::Pattern`] when the regex toggle is on and the text is not a valid expression.
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

    /// The text that replaces the match at `found` in `haystack`: `with` as typed, or with the
    /// regex toggle on, `with` with Monaco's `$1`, `$&` and `$$` filled from the match; with the
    /// preserve-case toggle on, cased as the match is.
    ///
    /// # Errors
    /// [`Error::Pattern`] when the regex toggle is on and the text is not a valid expression.
    pub fn replacement(
        &self,
        haystack: &str,
        found: &Range<usize>,
        with: &str,
    ) -> Result<String, Error> {
        let filled = self.filled(haystack, found, with)?;
        if !self.options.has(FindOption::PreserveCase) {
            return Ok(filled);
        }
        let matched = haystack.get(found.clone()).unwrap_or("");
        Ok(case_preserved(matched, &filled))
    }

    fn filled(
        &self,
        haystack: &str,
        found: &Range<usize>,
        with: &str,
    ) -> Result<String, Error> {
        if !self.options.has(FindOption::Regex) {
            return Ok(with.to_owned());
        }
        let pattern = self.pattern()?;
        let from_match = haystack.get(found.start..).unwrap_or("");
        let rest = haystack.get(found.end..).unwrap_or("");
        let replaced = pattern.replacen(from_match, 1, rust_template(with).as_str());
        Ok(replaced.strip_suffix(rest).unwrap_or(&replaced).to_owned())
    }

    fn source(&self) -> String {
        if self.options.has(FindOption::Regex) {
            return self.text.clone();
        }
        regex::escape(&self.text)
    }

    fn pattern(&self) -> Result<Regex, Error> {
        let source = self.source();
        RegexBuilder::new(&source)
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
        ("^price", &[FindOption::Regex], &[(0, 5), (19, 24)]),
        ("p.ice", &[], &[]),
        ("", &[], &[]),
    ];

    const REPLACED: &str = "let total = 1;";
    type Replacement = (&'static str, Toggles, &'static str, &'static str);

    const REPLACE_CASES: &[Replacement] = &[
        ("(t)(otal)", &[FindOption::Regex], "$2$1", "otalt"),
        ("(t)(otal)", &[FindOption::Regex], "[$&]", "[total]"),
        ("(t)(otal)", &[FindOption::Regex], "$1x", "tx"),
        ("total", &[], "$1", "$1"),
        ("TOTAL", &[FindOption::PreserveCase], "sum", "sum"),
        (
            "(t)otal",
            &[FindOption::Regex, FindOption::PreserveCase],
            "$1x",
            "tx",
        ),
    ];

    const CASED: &str = "let Total = TOTAL_SUM;";
    type Preserved = (Range<usize>, &'static str, &'static str);
    const PRESERVED_CASES: &[Preserved] =
        &[(4..9, "sum", "Sum"), (12..21, "grand_net", "GRAND_NET")];

    #[test]
    fn a_replacement_under_preserve_case_takes_the_case_of_what_it_replaces() {
        let query = FindQuery::new("x", FindOptions::of(&[FindOption::PreserveCase]));
        for (found, with, expected) in PRESERVED_CASES {
            let replaced = query.replacement(CASED, found, with);
            assert_eq!(replaced.ok().as_deref(), Some(*expected), "{with:?}");
        }
    }

    #[test]
    fn a_query_matches_as_its_toggles_say() {
        for (query, on, spans) in MATCH_CASES {
            let expected: Vec<Range<usize>> =
                spans.iter().map(|(start, end)| *start..*end).collect();
            let found = FindQuery::new(query, FindOptions::of(on)).matches(MATCHED);
            assert_eq!(found.ok(), Some(expected), "{query:?} {on:?}");
        }
    }

    #[test]
    fn a_replacement_fills_monaco_s_groups_only_under_the_regex_toggle() {
        for (query, on, with, expected) in REPLACE_CASES {
            let query = FindQuery::new(query, FindOptions::of(on));
            let replaced = query.replacement(REPLACED, &(4..9), with);
            assert_eq!(replaced.ok().as_deref(), Some(*expected), "{with:?}");
        }
    }

    #[test]
    fn an_invalid_expression_is_reported_not_matched() {
        let query = FindQuery::new("(", FindOptions::of(&[FindOption::Regex]));
        assert!(matches!(query.matches("("), Err(Error::Pattern { .. })));
    }
}
