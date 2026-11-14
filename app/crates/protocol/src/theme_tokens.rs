use std::collections::BTreeMap;

use serde::{Deserialize, Deserializer, Serialize};

use crate::error::{Error, Result};
use crate::string_id::check_text;

/// Most tokens a theme holds.
pub const MAX_THEME_TOKENS: usize = 256;

const MAX_VALUE_BYTES: usize = 128;

/// The design tokens a pane styles itself with: token name (`color-bg`, `radius.sm`) to CSS
/// value (`#1a1b1e`, `6px`). Names are lower case words, values are at most 128 bytes without
/// control characters, and there are at most [`MAX_THEME_TOKENS`] tokens; the limits hold for
/// values read from the wire too.
#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize)]
#[serde(transparent)]
pub struct ThemeTokens(BTreeMap<String, String>);

impl ThemeTokens {
    /// Adds or replaces one token.
    ///
    /// # Errors
    /// An [`Error`] when the name or value breaks its rules or the theme would pass
    /// [`MAX_THEME_TOKENS`] tokens.
    pub fn with(mut self, name: impl Into<String>, value: impl Into<String>) -> Result<Self> {
        let (name, value) = (name.into(), value.into());
        check_text("theme token name", &name, |c| {
            c.is_ascii_lowercase() || c.is_ascii_digit() || matches!(c, '-' | '.')
        })?;
        check_value(&value)?;
        if self.0.len() >= MAX_THEME_TOKENS && !self.0.contains_key(&name) {
            return Err(Error::TooManyEntries {
                kind: "theme",
                max: MAX_THEME_TOKENS,
            });
        }
        self.0.insert(name, value);
        Ok(self)
    }

    /// Looks up one token.
    #[must_use]
    pub fn get(&self, name: &str) -> Option<&str> {
        self.0.get(name).map(String::as_str)
    }

    /// The number of tokens.
    #[must_use]
    pub fn len(&self) -> usize {
        self.0.len()
    }

    /// Whether there are no tokens.
    #[must_use]
    pub fn is_empty(&self) -> bool {
        self.0.is_empty()
    }
}

fn check_value(value: &str) -> Result<()> {
    if value.len() > MAX_VALUE_BYTES {
        return Err(Error::IdTooLong {
            kind: "theme token value",
            max: MAX_VALUE_BYTES,
        });
    }
    match value.chars().find(|c| c.is_control()) {
        Some(found) => Err(Error::InvalidIdCharacter {
            kind: "theme token value",
            found,
        }),
        None => Ok(()),
    }
}

impl<'de> Deserialize<'de> for ThemeTokens {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> std::result::Result<Self, D::Error> {
        BTreeMap::<String, String>::deserialize(deserializer)?
            .into_iter()
            .try_fold(Self::default(), |tokens, (name, value)| {
                tokens.with(name, value)
            })
            .map_err(serde::de::Error::custom)
    }
}

#[cfg(test)]
#[path = "theme_tokens_tests.rs"]
mod tests;
