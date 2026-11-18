use http_body_util::{BodyExt, Empty, LengthLimitError, Limited};
use hyper::body::Bytes;
use hyper::client::conn::http1;
use hyper::header::HOST;
use hyper::{Method, Request};
use hyper_util::rt::TokioIo;
use tokio_util::task::AbortOnDropHandle;

use crate::dial::Dial;
use crate::error::Error;

fn failed(
    target: &str,
    source: impl std::error::Error + Send + Sync + 'static,
) -> Error {
    Error::Exchange {
        target: target.to_owned(),
        source: Box::new(source),
    }
}

fn body_failure(
    target: &str,
    limit: usize,
    error: Box<dyn std::error::Error + Send + Sync>,
) -> Error {
    match error.downcast::<LengthLimitError>() {
        Ok(_) => Error::BodyTooLarge {
            target: target.to_owned(),
            limit,
        },
        Err(other) => Error::Exchange {
            target: target.to_owned(),
            source: other,
        },
    }
}

pub(crate) async fn get<D: Dial>(
    dial: &D,
    target: &str,
    limit: usize,
) -> Result<Vec<u8>, Error> {
    let stream = dial.dial().await.map_err(|source| Error::Dial { source })?;
    let (mut sender, connection) = http1::handshake(TokioIo::new(stream))
        .await
        .map_err(|e| failed(target, e))?;
    let _driver = AbortOnDropHandle::new(tokio::spawn(connection));
    let request = Request::builder()
        .method(Method::GET)
        .uri(target)
        .header(HOST, "localhost")
        .body(Empty::<Bytes>::new())
        .map_err(|e| failed(target, e))?;
    let response = sender
        .send_request(request)
        .await
        .map_err(|e| failed(target, e))?;
    let status = response.status();
    status
        .is_success()
        .then_some(())
        .ok_or_else(|| Error::Status {
            target: target.to_owned(),
            status: status.as_u16(),
        })?;
    let body = Limited::new(response.into_body(), limit)
        .collect()
        .await
        .map_err(|e| body_failure(target, limit, e))?;
    Ok(body.to_bytes().to_vec())
}

#[cfg(test)]
mod tests {
    use std::io::ErrorKind;

    use super::*;
    use crate::test_support::fake_daemon::FakeDaemon;

    #[tokio::test]
    async fn a_successful_answer_is_returned_whole() {
        let daemon = FakeDaemon::answering(|_, target| (200, format!("echo {target}")));
        let body = get(&daemon, "/v0/health", 1024).await.unwrap();
        assert_eq!(body, b"echo /v0/health");
        assert_eq!(daemon.dialled(), 1, "one dial per request");
    }

    type Check = fn(&Error) -> bool;
    type Case = (&'static str, FakeDaemon, &'static str, usize, Check);

    fn refused(error: &Error) -> bool {
        matches!(error, Error::Dial { source } if source.kind() == ErrorKind::ConnectionRefused)
    }

    fn hung_up(error: &Error) -> bool {
        matches!(error, Error::Exchange { .. }) && !error.is_descriptor_exhaustion()
    }

    fn refused_status(error: &Error) -> bool {
        matches!(error, Error::Status { status: 503, target } if target == "/v0/projects")
    }

    fn too_large(error: &Error) -> bool {
        matches!(error, Error::BodyTooLarge { limit: 10, .. })
    }

    fn failure_cases() -> [Case; 4] {
        [
            (
                "status that is not success",
                FakeDaemon::answering(|_, _| (503, "{}".into())),
                "/v0/projects",
                1024,
                refused_status,
            ),
            (
                "answer over the limit is too large, not a broken connection",
                FakeDaemon::answering(|_, _| (200, "x".repeat(100))),
                "/big",
                10,
                too_large,
            ),
            (
                "dial keeps the os error that caused it",
                FakeDaemon::answering(|_, _| (200, String::new()))
                    .refusing(|_| Some(std::io::Error::from(ErrorKind::ConnectionRefused))),
                "/v0/health",
                10,
                refused,
            ),
            (
                "hang up without answering is an exchange error, not a local failure",
                FakeDaemon::answering(|_, _| (0, String::new())),
                "/v0/health",
                10,
                hung_up,
            ),
        ]
    }

    async fn assert_failures_are_named() {
        for (name, daemon, target, limit, check) in failure_cases() {
            let error = get(&daemon, target, limit).await.unwrap_err();
            assert!(check(&error), "{name}: {error:?}");
        }
    }

    #[tokio::test]
    async fn a_failed_exchange_is_an_error_naming_what_went_wrong() {
        assert_failures_are_named().await;
    }
}
