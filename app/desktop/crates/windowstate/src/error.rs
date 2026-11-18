use std::path::PathBuf;

/// Why the window state could not be read or saved.
#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum Error {
    /// The state file cannot be read or written.
    #[error("window state file {path}: {source}")]
    Io {
        /// The state file.
        path: PathBuf,
        /// The failure.
        #[source]
        source: std::io::Error,
    },
    /// The state file is not valid JSON of the expected shape.
    #[error("window state file {path} is unreadable: {source}")]
    Corrupt {
        /// The state file.
        path: PathBuf,
        /// The failure.
        #[source]
        source: serde_json::Error,
    },
    /// The state file holds bounds that are not finite numbers or not positive sizes.
    #[error("window state file {path} holds bounds that are not finite")]
    OutOfRange {
        /// The state file.
        path: PathBuf,
    },
}
