//! Identifier of a project.

use std::sync::Arc;

/// Opaque identifier handed out by the data source.
#[derive(Debug, Clone, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub struct ProjectId(Arc<str>);

impl From<&str> for ProjectId {
    fn from(value: &str) -> Self {
        Self(Arc::from(value))
    }
}

impl From<String> for ProjectId {
    fn from(value: String) -> Self {
        Self(Arc::from(value))
    }
}

impl ProjectId {
    /// The identifier as text.
    pub fn as_str(&self) -> &str {
        &self.0
    }
}

#[cfg(test)]
#[path = "project_id_tests.rs"]
mod tests;
