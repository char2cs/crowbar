use thiserror::Error;

use crate::host_id::HostId;

/// Everything that can go wrong in this crate and through [`DaemonTransport`](crate::DaemonTransport).
#[derive(Debug, Error)]
#[non_exhaustive]
pub enum Error {
    /// An identifier was empty.
    #[error("{kind} is empty")]
    EmptyId {
        /// Which identifier was being parsed.
        kind: &'static str,
    },
    /// An identifier was longer than the allowed maximum.
    #[error("{kind} is longer than {max} bytes")]
    IdTooLong {
        /// Which identifier was being parsed.
        kind: &'static str,
        /// Maximum length in bytes.
        max: usize,
    },
    /// An identifier contains a character outside its alphabet.
    #[error("{kind} contains invalid character {found:?}")]
    InvalidIdCharacter {
        /// Which identifier was being parsed.
        kind: &'static str,
        /// The offending character.
        found: char,
    },
    /// An identifier does not have the required shape.
    #[error("{kind} {reason}")]
    MalformedId {
        /// Which identifier was being parsed.
        kind: &'static str,
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
    UnknownHost(HostId),
    /// The daemon or remote host cannot be reached.
    #[error("host unavailable: {0}")]
    HostUnavailable(String),
    /// The request was refused by policy.
    #[error("denied: {0}")]
    Denied(String),
    /// The daemon did not answer within the budget.
    #[error("timed out")]
    Timeout,
}

/// Result alias using the crate [`enum@Error`].
pub type Result<T> = std::result::Result<T, Error>;
