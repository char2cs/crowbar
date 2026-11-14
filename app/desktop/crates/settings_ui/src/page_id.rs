//! The stable id of a settings page.

use std::fmt;

/// A page id such as `appearance`; the deep-link target of the modal. Extension pages start with `ext.<id>`.
#[derive(Debug, Clone, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub struct PageId(String);

impl PageId {
    /// A page id named `id`.
    pub fn new(id: impl Into<String>) -> Self {
        Self(id.into())
    }

    /// The id as text.
    pub fn as_str(&self) -> &str {
        &self.0
    }
}

impl From<&str> for PageId {
    fn from(id: &str) -> Self {
        Self::new(id)
    }
}

impl fmt::Display for PageId {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str(&self.0)
    }
}

#[cfg(test)]
#[path = "page_id_tests.rs"]
mod tests;
