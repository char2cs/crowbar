//! What to search for.

use crate::find::search_options::SearchOptions;

const REGEX_SPECIALS: &str = "\\.+*?()|[]{}^$#&-~";
const ASCII_WORD_BOUNDARY: &str = "(?-u:\\b)";

/// Search text and switches.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct SearchQuery {
    /// The text, or a regular expression with [`SearchOptions::REGEX`].
    pub text: String,
    /// How to read it.
    pub options: SearchOptions,
}

impl SearchQuery {
    pub(crate) fn pattern(&self) -> String {
        let bounded = self.bounded(self.body());
        if self.options.contains(SearchOptions::CASE_SENSITIVE) {
            return bounded;
        }
        format!("(?i){bounded}")
    }

    fn body(&self) -> String {
        if self.options.contains(SearchOptions::REGEX) {
            return self.text.clone();
        }
        escape(&self.text)
    }

    fn bounded(
        &self,
        body: String,
    ) -> String {
        if self.options.contains(SearchOptions::WHOLE_WORD) {
            return format!("{ASCII_WORD_BOUNDARY}{body}{ASCII_WORD_BOUNDARY}");
        }
        body
    }
}

fn escape(text: &str) -> String {
    text.chars()
        .flat_map(|character| {
            let special = REGEX_SPECIALS.contains(character);
            special
                .then_some('\\')
                .into_iter()
                .chain(std::iter::once(character))
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    fn query(
        text: &str,
        options: SearchOptions,
    ) -> SearchQuery {
        SearchQuery {
            text: text.into(),
            options,
        }
    }

    #[test]
    fn patterns_follow_the_options() {
        let cases = [
            (query("a.b", SearchOptions::NONE), "(?i)a\\.b"),
            (query("a.b", SearchOptions::CASE_SENSITIVE), "a\\.b"),
            (
                query(
                    "a.b",
                    SearchOptions::REGEX.with(SearchOptions::CASE_SENSITIVE),
                ),
                "a.b",
            ),
            (
                query(
                    "w",
                    SearchOptions::WHOLE_WORD.with(SearchOptions::CASE_SENSITIVE),
                ),
                "(?-u:\\b)w(?-u:\\b)",
            ),
        ];
        for (query, expected) in cases {
            assert_eq!(query.pattern(), expected);
        }
    }
}
