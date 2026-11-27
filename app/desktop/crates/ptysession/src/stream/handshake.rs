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

    #[test]
    fn header_values_reject_control_characters() {
        for (value, valid) in HEADER_CASES {
            assert_eq!(header(value).is_ok(), *valid, "{value:?}");
        }
    }

    #[tokio::test]
    async fn a_finished_attempt_returns_its_value_or_a_stream_error() {
        let done = within(std::future::ready(Ok::<u8, std::fmt::Error>(7))).await;
        assert_eq!(done.unwrap(), 7);
        let failed = within(std::future::ready(Err::<u8, _>(std::fmt::Error))).await;
        assert!(matches!(failed, Err(Error::Stream(_))));
    }
}
