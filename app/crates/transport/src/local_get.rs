use http::{HeaderValue, Method, Request, Response, header};
use tokio::net::UnixStream;

use crate::error::Error;
use crate::http_conn::exchange;

/// One HTTP/1.1 `GET` of `target` (path and query) over an already connected daemon socket,
/// buffering at most `limit` bytes of the answer. Connecting is the caller's, so it can tell a
/// daemon that does not answer from a process that has no descriptors left to dial with.
///
/// # Errors
/// [`Error::Http`], [`Error::TooLarge`] or [`Error::Body`] when the exchange fails.
pub async fn local_get(
    stream: UnixStream,
    target: &str,
    limit: usize,
) -> Result<Response<Vec<u8>>, Error> {
    let mut request = Request::builder()
        .method(Method::GET)
        .uri(target)
        .body(Vec::new())?;
    request
        .headers_mut()
        .insert(header::HOST, HeaderValue::from_static("localhost"));
    exchange(Box::new(stream), request, limit).await
}

#[cfg(test)]
#[path = "local_get_tests.rs"]
mod tests;
