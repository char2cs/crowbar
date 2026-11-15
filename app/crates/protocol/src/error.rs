use thiserror::Error;

/// Everything that can go wrong in this crate and through [`DaemonTransport`](crate::DaemonTransport).
#[derive(Debug, Error)]
#[non_exhaustive]
pub enum Error {
    /// A value was empty.
    #[error("{kind} is empty")]
    EmptyId {
        /// Which value was being parsed.
        kind: &'static str,
    },
    /// A value was longer than the allowed maximum.
    #[error("{kind} is longer than {max} bytes")]
    IdTooLong {
        /// Which value was being parsed.
        kind: &'static str,
        /// Maximum length in bytes.
        max: usize,
    },
    /// A value contains a character outside its alphabet.
    #[error("{kind} contains invalid character {found:?}")]
    InvalidIdCharacter {
        /// Which value was being parsed.
        kind: &'static str,
        /// The offending character.
        found: char,
    },
    /// A value does not have the required shape.
    #[error("{kind} {reason}")]
    MalformedId {
        /// Which value was being parsed.
        kind: &'static str,
        /// What is wrong with it.
        reason: &'static str,
    },
    /// A map holds more entries than allowed.
    #[error("{kind} holds more than {max} entries")]
    TooManyEntries {
        /// Which map was being built.
        kind: &'static str,
        /// Maximum number of entries.
        max: usize,
    },
    /// A daemon request is malformed.
    #[error("invalid request: {reason}")]
    InvalidRequest {
        /// What is wrong with it.
        reason: &'static str,
    },
    /// Pane state is larger than [`MAX_STATE_BYTES`](crate::MAX_STATE_BYTES).
    #[error("pane state is {size} bytes, the limit is {limit}")]
    StateTooLarge {
        /// Serialized size in bytes.
        size: usize,
        /// The enforced limit.
        limit: usize,
    },
    /// A frame is larger than [`MAX_FRAME_BYTES`](crate::MAX_FRAME_BYTES).
    #[error("frame is {size} bytes, the limit is {limit}")]
    FrameTooLarge {
        /// Size in bytes.
        size: usize,
        /// The enforced limit.
        limit: usize,
    },
    /// JSON could not be produced or parsed.
    #[error("malformed json: {0}")]
    Json(#[from] serde_json::Error),
    /// A frame names an SDK major version this build does not speak.
    #[error("unsupported sdk version {found:?}, expected {expected}")]
    UnsupportedSdk {
        /// The version found in the frame, if any.
        found: Option<u64>,
        /// The version this build speaks.
        expected: u32,
    },
    /// The host id is not known to the transport.
    #[error("unknown host {0}")]
    UnknownHost(String),
    /// The daemon or remote host cannot be reached; the cause is the transport's own error.
    #[error("host unavailable: {0}")]
    HostUnavailable(#[source] Box<dyn std::error::Error + Send + Sync>),
    /// The request was refused by policy; the reason is fixed text, safe to show to a page.
    #[error("denied: {0}")]
    Denied(&'static str),
    /// The transport stopped before it answered the request.
    #[error("the transport stopped before answering")]
    Interrupted,
    /// The daemon did not answer within the budget.
    #[error("timed out")]
    Timeout,
}

/// Result alias using the crate [`enum@Error`].
pub type Result<T> = std::result::Result<T, Error>;
