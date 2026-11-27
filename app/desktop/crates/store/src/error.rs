use std::path::PathBuf;

#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum Error {
    #[error(transparent)]
    Daemon(#[from] daemonapi::Error),
    #[error("shell data: stream: {0}")]
    Stream(String),
    #[error("shell data: {path}: {source}")]
    File {
        path: PathBuf,
        #[source]
        source: std::io::Error,
    },
    #[error("shell data: settings writer: {0}")]
    Writer(#[source] std::io::Error),
    #[error("shell data: {path} is not valid JSON: {source}")]
    Json {
        path: PathBuf,
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
            data: Box::default(),
        });
        let stream = Error::Stream("closed".into());
        assert_eq!(refused.reason(), "name taken");
        assert_eq!(stream.reason(), stream.to_string());
    }
}
