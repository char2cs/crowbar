//! Errors of the sidebar crate.

use std::path::PathBuf;

/// Why a command or a preferences operation failed.
#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum Error {
    /// The data source declined a command; the reason is shown next to the input that sent it.
    #[error("sidebar: command refused: {reason}")]
    Refused {
        /// The text for the user.
        reason: String,
    },
    /// The preferences file could not be read.
    #[error("sidebar: read preferences {path}: {source}")]
    ReadPreferences {
        /// The file.
        path: PathBuf,
        /// The cause.
        #[source]
        source: std::io::Error,
    },
    /// The preferences file could not be written.
    #[error("sidebar: write preferences {path}: {source}")]
    WritePreferences {
        /// The file.
        path: PathBuf,
        /// The cause.
        #[source]
        source: std::io::Error,
    },
    /// The preferences file holds invalid JSON.
    #[error("sidebar: parse preferences {path}: {source}")]
    ParsePreferences {
        /// The file.
        path: PathBuf,
        /// The cause.
        #[source]
        source: serde_json::Error,
    },
}
