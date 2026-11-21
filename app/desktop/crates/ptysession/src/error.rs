//! Why a terminal session could not start.

/// A failure while creating or attaching a terminal session.
#[derive(Debug, thiserror::Error)]
pub enum Error {
    /// The chat id cannot be part of a daemon path.
    #[error("terminal session: {0:?} is not a chat id")]
    ChatId(String),
    /// The daemon or the transport refused the request.
    #[error("terminal session: {0}")]
    Daemon(String),
    /// The daemon refused the request or answered something that is not its envelope.
    #[error("terminal session: {0}")]
    Api(#[from] daemonapi::Error),
    /// The daemon answered without a session id.
    #[error("terminal session: the daemon answered without a session id")]
    NoSessionId,
    /// A stage of the connection did not finish in time.
    #[error("terminal session: timed out {stage}")]
    TimedOut {
        /// What was being attempted.
        stage: &'static str,
        /// The elapsed deadline.
        #[source]
        source: tokio::time::error::Elapsed,
    },
    /// The transport dropped the request before answering.
    #[error("terminal session: the transport stopped")]
    TransportStopped(#[source] tokio::sync::oneshot::error::RecvError),
    /// The websocket could not be opened.
    #[error("terminal session: stream: {0}")]
    Stream(String),
}

impl Error {
    pub(crate) fn words(&self) -> String {
        let Self::Api(daemonapi::Error::Refused { message, .. }) = self else {
            return self.to_string();
        };
        message.clone()
    }
}

/// Result of a session operation.
pub type Result<T> = std::result::Result<T, Error>;

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn errors_name_what_failed() {
        assert_eq!(
            Error::NoSessionId.to_string(),
            "terminal session: the daemon answered without a session id"
        );
        assert_eq!(
            Error::NoSessionId.words(),
            Error::NoSessionId.to_string(),
            "an error the daemon did not word speaks for itself"
        );
    }
}
