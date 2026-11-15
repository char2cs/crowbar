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
    if !status.is_success() {
        return Err(Error::Status {
            target: target.to_owned(),
            status: status.as_u16(),
        });
    }
    let body = Limited::new(response.into_body(), limit)
        .collect()
        .await
        .map_err(|e| match e.downcast::<LengthLimitError>() {
            Ok(_) => Error::BodyTooLarge {
                target: target.to_owned(),
                limit,
            },
            Err(other) => Error::Exchange {
                target: target.to_owned(),
                source: other,
            },
        })?;
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

    #[tokio::test]
    async fn a_status_that_is_not_success_is_an_error_naming_it() {
        let daemon = FakeDaemon::answering(|_, _| (503, "{}".into()));
        let error = get(&daemon, "/v0/projects", 1024).await.unwrap_err();
        assert!(
            matches!(&error, Error::Status { status: 503, target } if target == "/v0/projects"),
            "{error:?}"
        );
    }

    #[tokio::test]
    async fn an_answer_over_the_limit_is_refused_as_too_large_not_as_a_broken_connection() {
        let daemon = FakeDaemon::answering(|_, _| (200, "x".repeat(100)));
        let error = get(&daemon, "/big", 10).await.unwrap_err();
        assert!(
            matches!(error, Error::BodyTooLarge { limit: 10, .. }),
            "{error:?}"
        );
    }

    #[tokio::test]
    async fn a_failed_dial_keeps_the_os_error_that_caused_it() {
        let daemon = FakeDaemon::answering(|_, _| (200, String::new()))
            .refusing(|_| Some(std::io::Error::from(ErrorKind::ConnectionRefused)));
        let error = get(&daemon, "/v0/health", 10).await.unwrap_err();
        assert!(
            matches!(&error, Error::Dial { source } if source.kind() == ErrorKind::ConnectionRefused),
            "{error:?}"
        );
    }

    #[tokio::test]
    async fn a_daemon_that_hangs_up_without_answering_is_an_exchange_error() {
        let daemon = FakeDaemon::answering(|_, _| (0, String::new()));
        let error = get(&daemon, "/v0/health", 10).await.unwrap_err();
        assert!(matches!(error, Error::Exchange { .. }), "{error:?}");
        assert!(!error.is_descriptor_exhaustion(), "not a local failure");
    }
}
