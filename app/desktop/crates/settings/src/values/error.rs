//! Errors of the settings modal and its backends.

use crate::values::key::Key;

/// What can go wrong building a schema or talking to a backend.
#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum Error {
    /// The schema repeats a page id or a key.
    #[error("settings schema: {what} `{name}` is declared twice")]
    Duplicate {
        /// Whether a `page` or a `key` repeats.
        what: &'static str,
        /// The repeated name.
        name: String,
    },
    /// An extension declared a page or key outside its namespace.
    #[error("settings schema: `{name}` is outside the namespace `ext.{extension}`")]
    OutsideNamespace {
        /// The extension id.
        extension: String,
        /// The offending page id or key.
        name: String,
    },
    /// The backend refused a write.
    #[error("setting {key} was not saved: {reason}")]
    Rejected {
        /// The key that was written.
        key: Key,
        /// The backend's explanation.
        reason: String,
    },
    /// The backend could not be reached.
    #[error("settings backend is unavailable: {0}")]
    Unavailable(String),
}
