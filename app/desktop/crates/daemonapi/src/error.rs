#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum Error {
    #[error("daemon api: request {path}: {source}")]
    Request {
        path: String,
        #[source]
        source: protocol::Error,
    },
    #[error("{message}")]
    Refused {
        path: String,
        status: u16,
        code: Option<String>,
        message: String,
        data: Box<serde_json::Value>,
    },
    #[error("daemon api: {path} answered a body that does not parse: {source}")]
    Body {
        path: String,
        #[source]
        source: serde_json::Error,
    },
    #[error("daemon api: {path} answered data of another shape: {source}")]
    Json {
        path: String,
        #[source]
        source: serde_json::Error,
    },
    #[error("daemon api: stream: {0}")]
    Stream(String),
}

pub type Result<T> = std::result::Result<T, Error>;
