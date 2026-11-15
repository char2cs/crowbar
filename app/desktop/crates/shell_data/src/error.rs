//! Errors of the shell data layer.

use std::path::PathBuf;

/// Why a daemon request, a stream or a settings file failed.
#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum Error {
    /// The request never produced an answer: the host is unknown or unreachable.
    #[error("shell data: request {path}: {source}")]
    Request {
        /// The route asked for.
        path: String,
        /// The cause.
        #[source]
        source: protocol::Error,
    },
    /// The daemon answered with a refusal.
    #[error("{message}")]
    Refused {
        /// The route asked for.
        path: String,
        /// The HTTP status.
        status: u16,
        /// The daemon's error code, when it sent one.
        code: Option<String>,
        /// The daemon's explanation, fit to show.
        message: String,
    },
    /// The answer was not the envelope or shape the route promises.
    #[error("shell data: {path} answered a body that does not parse: {source}")]
    Body {
        /// The route asked for.
        path: String,
        /// The cause.
        #[source]
        source: serde_json::Error,
    },
    /// The stream could not be opened or broke.
    #[error("shell data: stream: {0}")]
    Stream(String),
    /// A file of the shell could not be read or written.
    #[error("shell data: {path}: {source}")]
    File {
        /// The file.
        path: PathBuf,
        /// The cause.
        #[source]
        source: std::io::Error,
    },
    /// A file of the shell holds invalid JSON.
    #[error("shell data: {path} is not valid JSON: {source}")]
    Json {
        /// The file.
        path: PathBuf,
        /// The cause.
        #[source]
        source: serde_json::Error,
    },
}

/// The result of this crate.
pub type Result<T> = std::result::Result<T, Error>;
