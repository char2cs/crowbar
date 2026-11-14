use http_body_util::{BodyExt, Empty, LengthLimitError, Limited};
use hyper::body::Bytes;
use hyper::client::conn::http1;
use hyper::header::HOST;
use hyper::{Method, Request};
use hyper_util::rt::TokioIo;
use tokio_util::task::AbortOnDropHandle;

use crate::dial::Dial;
use crate::error::Error;

fn failed(target: &str, source: impl std::error::Error + Send + Sync + 'static) -> Error {
    Error::Exchange {
        target: target.to_owned(),
        source: Box::new(source),
    }
}

pub(crate) async fn get<D: Dial>(dial: &D, target: &str, limit: usize) -> Result<Vec<u8>, Error> {
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
#[path = "daemon_get_tests.rs"]
pub(crate) mod tests;
