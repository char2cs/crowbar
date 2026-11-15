//! Checks applied to typed text before it counts as valid.

use crate::issue::Issue;

/// A named check for a text field.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Validator {
    /// An http(s) address with a `%s` where the query goes.
    SearchTemplate,
}

impl Validator {
    /// Whether `text` is acceptable; empty text always is, because it means "unset".
    ///
    /// # Errors
    /// Returns the [`Issue`] found in a non-empty `text`.
    pub fn check(
        self,
        text: &str,
    ) -> Result<(), Issue> {
        match self {
            Self::SearchTemplate => check_template(text),
        }
    }
}

fn check_template(text: &str) -> Result<(), Issue> {
    let text = text.trim();
    if text.is_empty() {
        return Ok(());
    }
    let usable =
        has_web_scheme(text) && !text.chars().any(char::is_whitespace) && text.contains("%s");
    usable.then_some(()).ok_or(Issue::BadSearchTemplate)
}

fn has_web_scheme(text: &str) -> bool {
    ["https://", "http://"].iter().any(|scheme| {
        text.get(..scheme.len())
            .is_some_and(|head| head.eq_ignore_ascii_case(scheme))
            && text.len() > scheme.len()
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn search_template_rules() {
        const CASES: &[(&str, bool)] = &[
            ("", true),
            ("https://kagi.com/search?q=%s", true),
            ("http://example.com/?q=%s", true),
            ("https://kagi.com/search", false),
            ("ftp://example.com/%s", false),
            ("https://", false),
            ("https://a b/%s", false),
            ("kagi.com/%s", false),
            ("  https://kagi.com/search?q=%s  ", true),
            ("HTTPS://Kagi.com/?q=%s", true),
            ("   ", true),
            ("https://%s", true),
            ("é://x/%s", false),
        ];
        for &(text, valid) in CASES {
            assert_eq!(
                Validator::SearchTemplate.check(text).is_ok(),
                valid,
                "{text:?}"
            );
        }
    }
}
