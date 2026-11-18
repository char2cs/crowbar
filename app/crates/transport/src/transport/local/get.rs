use http::{HeaderValue, Method, Request, Response, header};
use tokio::io::{AsyncRead, AsyncWrite};

use crate::error::Error;
use crate::transport::http_conn::exchange;

/// One HTTP/1.1 `GET` of `target` (path and query) over an already connected daemon stream,
/// buffering at most `limit` bytes of the answer. Connecting is the caller's, so it can tell a
/// daemon that does not answer from a process that has no descriptors left to dial with.
///
/// # Errors
/// [`Error::Exchange`], [`Error::TooLarge`] or [`Error::Body`] when the exchange fails.
pub async fn local_get<S>(
    stream: S,
    target: &str,
    limit: usize,
) -> Result<Response<Vec<u8>>, Error>
where
    S: AsyncRead + AsyncWrite + Unpin + Send + 'static,
{
    let mut request = Request::builder()
        .method(Method::GET)
        .uri(target)
        .body(Vec::new())
        .map_err(Error::bad_request)?;
    request
        .headers_mut()
        .insert(header::HOST, HeaderValue::from_static("localhost"));
    exchange(Box::new(stream), request, limit).await
}

#[cfg(test)]
mod tests {
    use tokio::io::{AsyncReadExt, AsyncWriteExt, DuplexStream, duplex};

    use super::*;

    const BUFFER: usize = 4096;

    async fn serve_once(
        mut stream: DuplexStream,
        reply: &'static str,
    ) -> String {
        let mut buf = vec![0u8; BUFFER];
        let read = stream.read(&mut buf).await.unwrap();
        stream.write_all(reply.as_bytes()).await.unwrap();
        String::from_utf8_lossy(&buf[..read]).into_owned()
    }

    #[tokio::test]
    async fn it_gets_the_target_and_returns_the_buffered_answer() {
        let (near, far) = duplex(BUFFER);
        let daemon = tokio::spawn(serve_once(
            far,
            "HTTP/1.1 200 OK\r\ncontent-length: 2\r\nconnection: close\r\n\r\nhi",
        ));
        let response = local_get(near, "/v0/health?x=1", 1024).await.unwrap();
        assert_eq!(response.status(), 200);
        assert_eq!(response.body(), b"hi");
        let seen = daemon.await.unwrap();
        assert!(seen.starts_with("GET /v0/health?x=1 HTTP/1.1"), "{seen}");
    }

    #[tokio::test]
    async fn an_answer_over_the_limit_is_refused() {
        let (near, far) = duplex(BUFFER);
        tokio::spawn(serve_once(
            far,
            "HTTP/1.1 200 OK\r\ncontent-length: 10\r\nconnection: close\r\n\r\n0123456789",
        ));
        let error = local_get(near, "/v0/x", 4).await.unwrap_err();
        assert!(matches!(error, Error::TooLarge), "{error}");
    }
}
