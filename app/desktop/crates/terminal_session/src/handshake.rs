//! Opening the relay websocket: bounded connects and header values.

use std::time::Duration;

use tokio::time::timeout;
use tokio_tungstenite::tungstenite::http::HeaderValue;

use crate::error::{Error, Result};

pub(crate) const CONNECT_TIMEOUT: Duration = Duration::from_secs(10);

pub(crate) async fn within<T, E>(
    attempt: impl Future<Output = std::result::Result<T, E>>
) -> Result<T>
where
    E: std::fmt::Display,
{
    let done = timeout(CONNECT_TIMEOUT, attempt)
        .await
        .map_err(|source| Error::TimedOut {
            stage: "connecting",
            source,
        })?;
    done.map_err(stream_error)
}

pub(crate) fn header(value: &str) -> Result<HeaderValue> {
    HeaderValue::from_str(value).map_err(stream_error)
}

pub(crate) fn stream_error(error: impl std::fmt::Display) -> Error {
    Error::Stream(error.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    const HEADER_CASES: &[(&str, bool)] = &[
        ("crowbar://local", true),
        ("crowbar.relay.v1, crowbar.token.abc", true),
        ("line\nbreak", false),
    ];

    fn succeeded() -> std::future::Ready<std::result::Result<u8, std::fmt::Error>> {
        std::future::ready(Ok(7))
    }

    fn refused() -> std::future::Ready<std::result::Result<u8, std::fmt::Error>> {
        std::future::ready(Err(std::fmt::Error))
    }

    #[test]
    fn header_values_reject_control_characters() {
        for (value, valid) in HEADER_CASES {
            assert_eq!(header(value).is_ok(), *valid, "{value:?}");
        }
    }

    #[tokio::test]
    async fn a_finished_attempt_returns_its_value_or_a_stream_error() {
        let done = within(succeeded()).await;
        assert_eq!(done.unwrap(), 7);
        let failed = within(refused()).await;
        assert!(matches!(failed, Err(Error::Stream(_))));
    }
}
