//! The name of a setting.

use std::fmt;

/// A setting key such as `uiFontSize`.
#[derive(Debug, Clone, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub struct Key(String);

impl Key {
    /// A key named `name`.
    pub fn new(name: impl Into<String>) -> Self {
        Self(name.into())
    }

    /// The key as text.
    pub fn as_str(&self) -> &str {
        &self.0
    }
}

impl From<&str> for Key {
    fn from(name: &str) -> Self {
        Self::new(name)
    }
}

impl fmt::Display for Key {
    fn fmt(
        &self,
        formatter: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        formatter.write_str(&self.0)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn displays_as_its_name() {
        assert_eq!(Key::from("tabSize").to_string(), "tabSize");
        assert_eq!(Key::from("tabSize").as_str(), "tabSize");
    }
}
