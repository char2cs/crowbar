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
    /// Pane state is larger than 64 KiB.
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

impl Error {
    pub(crate) fn too_long(
        kind: &'static str,
        max: usize,
    ) -> Self {
        Self::IdTooLong { kind, max }
    }

    pub(crate) fn malformed(
        kind: &'static str,
        reason: &'static str,
    ) -> Self {
        Self::MalformedId { kind, reason }
    }

    pub(crate) fn too_many_entries(
        kind: &'static str,
        max: usize,
    ) -> Self {
        Self::TooManyEntries { kind, max }
    }

    pub(crate) fn state_too_large(
        size: usize,
        limit: usize,
    ) -> Self {
        Self::StateTooLarge { size, limit }
    }

    pub(crate) fn frame_too_large(
        size: usize,
        limit: usize,
    ) -> Self {
        Self::FrameTooLarge { size, limit }
    }

    pub(crate) fn invalid_request(reason: &'static str) -> Self {
        Self::InvalidRequest { reason }
    }

    pub(crate) fn unsupported_sdk(
        found: Option<u64>,
        expected: u32,
    ) -> Self {
        Self::UnsupportedSdk { found, expected }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn constructors_build_the_matching_variants() {
        let cases = [
            (Error::too_long("id", 3), "id is longer than 3 bytes"),
            (Error::malformed("id", "is odd"), "id is odd"),
            (
                Error::too_many_entries("params", 2),
                "params holds more than 2 entries",
            ),
            (
                Error::state_too_large(5, 4),
                "pane state is 5 bytes, the limit is 4",
            ),
            (
                Error::frame_too_large(5, 4),
                "frame is 5 bytes, the limit is 4",
            ),
            (Error::invalid_request("bad"), "invalid request: bad"),
        ];
        for (error, text) in cases {
            assert_eq!(error.to_string(), text);
        }
        assert!(matches!(
            Error::unsupported_sdk(None, 1),
            Error::UnsupportedSdk {
                found: None,
                expected: 1
            }
        ));
    }
}
