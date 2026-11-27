use thiserror::Error;

#[derive(Debug, Error)]
#[non_exhaustive]
pub enum Error {
    #[error("{kind} is empty")]
    EmptyId { kind: &'static str },
    #[error("{kind} is longer than {max} bytes")]
    IdTooLong { kind: &'static str, max: usize },
    #[error("{kind} contains invalid character {found:?}")]
    InvalidIdCharacter { kind: &'static str, found: char },
    #[error("{kind} {reason}")]
    MalformedId {
        kind: &'static str,
        reason: &'static str,
    },
    #[error("{kind} holds more than {max} entries")]
    TooManyEntries { kind: &'static str, max: usize },
    #[error("invalid request: {reason}")]
    InvalidRequest { reason: &'static str },
    #[error("pane state is {size} bytes, the limit is {limit}")]
    StateTooLarge { size: usize, limit: usize },
    #[error("frame is {size} bytes, the limit is {limit}")]
    FrameTooLarge { size: usize, limit: usize },
    #[error("malformed json: {0}")]
    Json(#[from] serde_json::Error),
    #[error("unsupported sdk version {found:?}, expected {expected}")]
    UnsupportedSdk { found: Option<u64>, expected: u32 },
    #[error("unknown host {0}")]
    UnknownHost(String),
    #[error("host unavailable: {0}")]
    HostUnavailable(#[source] Box<dyn std::error::Error + Send + Sync>),
    #[error("denied: {0}")]
    Denied(&'static str),
    #[error("the transport stopped before answering")]
    Interrupted,
    #[error("timed out")]
    Timeout,
}

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
