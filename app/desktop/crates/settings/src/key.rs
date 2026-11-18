//! The name of a setting.

use std::fmt;

/// A setting key such as `uiFontSize` or `ext.git.autoFetch`; extension keys start with `ext.<id>.`.
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

    pub(crate) fn is_in(
        &self,
        namespace: &str,
    ) -> bool {
        self.0
            .strip_prefix(namespace)
            .is_some_and(|rest| rest.starts_with('.'))
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
    fn namespace_membership_needs_the_dot_boundary() {
        const CASES: &[(&str, &str, bool)] = &[
            ("ext.git.autoFetch", "ext.git", true),
            ("ext.gitlab.token", "ext.git", false),
            ("ext.git", "ext.git", false),
            ("uiFontSize", "ext.git", false),
        ];
        for &(key, namespace, expected) in CASES {
            assert_eq!(
                Key::new(key).is_in(namespace),
                expected,
                "{key} in {namespace}"
            );
        }
    }

    #[test]
    fn displays_as_its_name() {
        assert_eq!(Key::from("tabSize").to_string(), "tabSize");
        assert_eq!(Key::from("tabSize").as_str(), "tabSize");
    }
}
