use std::path::PathBuf;

/// Why diagnostics could not start.
#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum Error {
    /// The log directory cannot be created or the log file cannot be opened.
    #[error("cannot open the log file in {dir}: {source}")]
    LogFile {
        /// The log directory.
        dir: PathBuf,
        /// The failure.
        #[source]
        source: std::io::Error,
    },
    /// The diagnostics bundle cannot be written.
    #[error("cannot write the diagnostics bundle {path}: {source}")]
    Bundle {
        /// The bundle file.
        path: PathBuf,
        /// The failure.
        #[source]
        source: Box<dyn std::error::Error + Send + Sync>,
    },
    /// Another subscriber is already installed.
    #[error("a logging subscriber is already installed")]
    AlreadyInstalled(#[source] Box<dyn std::error::Error + Send + Sync>),
}
