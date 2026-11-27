#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("terminal session: {0:?} is not a chat id")]
    ChatId(String),
    #[error("terminal session: {0}")]
    Daemon(String),
    #[error("terminal session: {0}")]
    Api(#[from] daemonapi::Error),
    #[error("terminal session: the daemon answered without a session id")]
    NoSessionId,
    #[error("terminal session: timed out {stage}")]
    TimedOut {
        stage: &'static str,
        #[source]
        source: tokio::time::error::Elapsed,
    },
    #[error("terminal session: the transport stopped")]
    TransportStopped(#[source] tokio::sync::oneshot::error::RecvError),
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
