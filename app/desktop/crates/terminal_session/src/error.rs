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
    /// The daemon answered without a session id.
    #[error("terminal session: the daemon answered without a session id")]
    NoSessionId,
    /// The websocket could not be opened.
    #[error("terminal session: stream: {0}")]
    Stream(String),
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
    }
}
