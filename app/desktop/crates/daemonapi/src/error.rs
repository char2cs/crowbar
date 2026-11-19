//! Errors of a daemon request or stream.

/// Why a daemon request or a stream failed.
#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum Error {
    /// The request never produced an answer: the host is unknown or unreachable.
    #[error("daemon api: request {path}: {source}")]
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
        /// The refusal's `data`, such as the work a delete was refused over; `null` when absent.
        data: Box<serde_json::Value>,
    },
    /// The answer was not the envelope or shape the route promises.
    #[error("daemon api: {path} answered a body that does not parse: {source}")]
    Body {
        /// The route asked for.
        path: String,
        /// The cause.
        #[source]
        source: serde_json::Error,
    },
    /// The `data` of an answer is not the type the route promises.
    #[error("daemon api: {path} answered data of another shape: {source}")]
    Json {
        /// The route asked for.
        path: String,
        /// The cause.
        #[source]
        source: serde_json::Error,
    },
    /// The stream could not be opened or broke.
    #[error("daemon api: stream: {0}")]
    Stream(String),
}

/// The result of this crate.
pub type Result<T> = std::result::Result<T, Error>;
