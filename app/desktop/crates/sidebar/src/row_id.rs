//! Identifier of a row.

use std::sync::Arc;

/// Opaque identifier handed out by the data source.
#[derive(Debug, Clone, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub struct RowId(Arc<str>);

impl From<&str> for RowId {
    fn from(value: &str) -> Self {
        Self(Arc::from(value))
    }
}

impl From<String> for RowId {
    fn from(value: String) -> Self {
        Self(Arc::from(value))
    }
}

impl RowId {
    /// The identifier as text.
    pub fn as_str(&self) -> &str {
        &self.0
    }
}

#[cfg(test)]
#[path = "row_id_tests.rs"]
mod tests;
