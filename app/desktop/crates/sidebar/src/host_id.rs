//! Identifier of a host.

use std::sync::Arc;

/// Opaque identifier handed out by the data source.
#[derive(Debug, Clone, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub struct HostId(Arc<str>);

impl From<&str> for HostId {
    fn from(value: &str) -> Self {
        Self(Arc::from(value))
    }
}

impl From<String> for HostId {
    fn from(value: String) -> Self {
        Self(Arc::from(value))
    }
}

impl HostId {
    /// The identifier as text.
    pub fn as_str(&self) -> &str {
        &self.0
    }
}

#[cfg(test)]
#[path = "host_id_tests.rs"]
mod tests;
