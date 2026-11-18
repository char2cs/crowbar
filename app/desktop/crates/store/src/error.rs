//! Errors of the shell data layer.

use std::path::PathBuf;

/// Why a daemon request, a stream or a settings file failed.
#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum Error {
    /// A request to a daemon or its stream failed.
    #[error(transparent)]
    Daemon(#[from] daemonapi::Error),
    /// The followed host or the row a request needs is not there.
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
    /// The settings database of the previous desktop app could not be read.
    #[error("shell data: legacy settings {path}: {source}")]
    Legacy {
        /// The database.
        path: PathBuf,
        /// The cause.
        #[source]
        source: rusqlite::Error,
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

impl Error {
    pub(crate) fn reason(&self) -> String {
        match self {
            Self::Daemon(daemonapi::Error::Refused { message, .. }) => message.clone(),
            other => other.to_string(),
        }
    }
}

/// The result of this crate.
pub type Result<T> = std::result::Result<T, Error>;

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_refusal_reads_as_the_daemons_words_and_anything_else_as_its_own_text() {
        let refused = Error::Daemon(daemonapi::Error::Refused {
            path: "/v0/x".into(),
            status: 409,
            code: None,
            message: "name taken".into(),
        });
        let stream = Error::Stream("closed".into());
        assert_eq!(refused.reason(), "name taken");
        assert_eq!(stream.reason(), stream.to_string());
    }
}
