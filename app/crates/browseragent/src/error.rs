#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum Error {
    #[error("unknown browser op {0:?}")]
    UnknownOp(String),
    #[error("{op}: {name} must be {expected}")]
    Argument {
        op: &'static str,
        name: &'static str,
        expected: &'static str,
    },
    #[error("{text:?} cannot be opened: {source}")]
    Address {
        text: String,
        #[source]
        source: browser::Error,
    },
    #[error("the daemon connection failed: {0}")]
    Stream(#[source] Box<dyn std::error::Error + Send + Sync>),
}
