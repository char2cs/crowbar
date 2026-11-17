/// Why a request of the daemon cannot be carried out.
#[derive(Debug, thiserror::Error, PartialEq, Eq)]
#[non_exhaustive]
pub enum Error {
    /// The operation is not one the browser tools define.
    #[error("unknown browser op {0:?}")]
    UnknownOp(String),
    /// An argument is missing or has the wrong type.
    #[error("{op}: {name} must be {expected}")]
    Argument {
        /// The operation.
        op: &'static str,
        /// The argument.
        name: &'static str,
        /// What it must be.
        expected: &'static str,
    },
    /// An address a browser pane does not load.
    #[error("{text:?} cannot be opened: {source}")]
    Address {
        /// The address as given.
        text: String,
        /// Why it is refused.
        #[source]
        source: browser_nav::Error,
    },
    /// The connection to the daemon failed.
    #[error("the daemon connection failed: {0}")]
    Stream(String),
}
