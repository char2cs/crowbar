//! The two matchers of the settings search: the index finds pages, the row text finds rows.
//! Spec: web/src/utils/search-match.ts (`scoreSearchQuery`) and
//! web/src/features/settings/lib/settings-row-search.ts.

use crate::schema::Row;
use crate::schema::SearchEntry;

#[derive(Debug, Clone, Default)]
pub(crate) struct Query {
    phrase: String,
    tokens: Vec<String>,
}

impl Query {
    pub(crate) fn new(text: &str) -> Self {
        Self {
            phrase: text.trim().to_lowercase(),
            tokens: normalized(text)
                .split_whitespace()
                .map(str::to_owned)
                .collect(),
        }
    }

    pub(crate) fn is_blank(&self) -> bool {
        self.phrase.is_empty()
    }

    pub(crate) fn finds_entry(
        &self,
        entry: &SearchEntry,
    ) -> bool {
        if self.tokens.is_empty() {
            return false;
        }
        let mut fields = vec![
            entry.label.as_str(),
            entry.description.as_str(),
            entry.section.as_str(),
        ];
        fields.extend(entry.keywords.iter().map(String::as_str));
        self.tokens
            .iter()
            .all(|token| fields.iter().any(|field| holds(field, token)))
    }

    pub(crate) fn finds_row(
        &self,
        row: &Row,
    ) -> bool {
        if self.is_blank() || row.label.to_lowercase().contains(&self.phrase) {
            return true;
        }
        row.description
            .as_deref()
            .is_some_and(|text| text.to_lowercase().contains(&self.phrase))
    }
}

fn normalized(text: &str) -> String {
    let lowered = text.to_lowercase();
    let spaced: String = lowered.chars().map(spaced_out).collect();
    spaced.split_whitespace().collect::<Vec<_>>().join(" ")
}

fn spaced_out(c: char) -> char {
    if c.is_ascii_alphanumeric() {
        return c;
    }
    ' '
}

fn compact(text: &str) -> String {
    normalized(text).replace(' ', "")
}

fn holds(
    field: &str,
    token: &str,
) -> bool {
    normalized(field).contains(token) || compact(field).contains(&compact(token))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::schema::Control;

    #[test]
    fn every_token_must_land_in_some_field_of_the_entry() {
        const CASES: &[(&str, bool)] = &[
            ("terminal", true),
            ("TYPO", true),
            ("size pixels", true),
            ("nerdfont", true),
            ("font-size", true),
            ("terminal zzz", false),
            ("   ", false),
        ];
        let entry = SearchEntry::new("Typography", "Terminal Font Size", "In pixels")
            .keywords(&["nerd font"]);
        for &(query, expected) in CASES {
            assert_eq!(Query::new(query).finds_entry(&entry), expected, "{query:?}");
        }
    }

    #[test]
    fn a_row_matches_the_whole_phrase_in_its_label_or_description() {
        const CASES: &[(&str, bool)] = &[
            ("", true),
            ("scroll", true),
            (" TERMINAL history ", true),
            ("history terminal", false),
            ("cursor", false),
        ];
        let row = Row::new("terminalScrollback", "Scrollback", Control::switch())
            .describe("How many lines of terminal history to keep");
        for &(query, expected) in CASES {
            assert_eq!(Query::new(query).finds_row(&row), expected, "{query:?}");
        }
    }
}
