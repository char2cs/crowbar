use bytes::Bytes;
use http::{HeaderName, HeaderValue, Request, Response, header};
use http_body_util::{BodyExt, Full, LengthLimitError, Limited};
use hyper::client::conn::http1::SendRequest;
use hyper_util::rt::TokioIo;

use crate::async_stream::BoxedStream;
use crate::error::Error;
use crate::response::PROXY_HEADER;

/// An HTTP/1.1 connection, kept alive between requests while the peer allows.
pub type Channel = SendRequest<Full<Bytes>>;

/// Starts HTTP/1.1 over `stream`; the connection task ends when the channel is dropped.
///
/// # Errors
/// [`Error::Http`] when the handshake fails.
pub async fn open_channel(stream: BoxedStream) -> Result<Channel, Error> {
    let (sender, connection) = hyper::client::conn::http1::handshake(TokioIo::new(stream)).await?;
    tokio::spawn(async move {
        connection.await.ok();
    });
    Ok(sender)
}

fn upstream_request(request: Request<Vec<u8>>) -> Result<Request<Full<Bytes>>, Error> {
    let target = request.uri().path_and_query().map_or_else(
        || request.uri().path().to_owned(),
        |pq| pq.as_str().to_owned(),
    );
    let (parts, body) = request.into_parts();
    let mut upstream = Request::builder().method(parts.method).uri(target);
    if let Some(headers) = upstream.headers_mut() {
        headers.extend(parts.headers);
        if !headers.contains_key(header::HOST) {
            headers.insert(header::HOST, HeaderValue::from_static("localhost"));
        }
    }
    Ok(upstream.body(Full::new(Bytes::from(body)))?)
}

/// Sends `request` (whose target is rewritten to its path and query) and buffers the answer up
/// to `body_limit` bytes. Any `x-crowbar-proxy` header the daemon sent is dropped: the marker is
/// the proxy's word alone.
///
/// # Errors
/// [`Error::Http`], [`Error::BadRequest`], [`Error::TooLarge`] or [`Error::Body`].
pub async fn send(
    channel: &mut Channel,
    request: Request<Vec<u8>>,
    body_limit: usize,
) -> Result<Response<Vec<u8>>, Error> {
    channel.ready().await?;
    let answer = channel.send_request(upstream_request(request)?).await?;
    let (parts, body) = answer.into_parts();
    let mut out = Response::new(collect_capped(body, body_limit).await?);
    *out.status_mut() = parts.status;
    let proxy = HeaderName::from_static(PROXY_HEADER);
    let relayed = parts
        .headers
        .into_iter()
        .filter(|(name, _)| name.as_ref() != Some(&proxy));
    out.headers_mut().extend(relayed);
    Ok(out)
}

/// Opens a channel on `stream` and sends one request.
///
/// # Errors
/// As [`send`] and [`open_channel`].
pub async fn exchange(
    stream: BoxedStream,
    request: Request<Vec<u8>>,
    body_limit: usize,
) -> Result<Response<Vec<u8>>, Error> {
    let mut channel = open_channel(stream).await?;
    send(&mut channel, request, body_limit).await
}

/// Buffers `body`, failing once it passes `limit`: counted as it streams in, so an endless body
/// never lands in memory.
///
/// # Errors
/// [`Error::TooLarge`] past the limit, [`Error::Body`] when the body fails.
pub async fn collect_capped<B>(body: B, limit: usize) -> Result<Vec<u8>, Error>
where
    B: hyper::body::Body,
    B::Error: Into<Box<dyn std::error::Error + Send + Sync>>,
{
    match Limited::new(body, limit).collect().await {
        Ok(collected) => Ok(collected.to_bytes().to_vec()),
        Err(e) if e.is::<LengthLimitError>() => Err(Error::TooLarge),
        Err(e) => Err(Error::Body(e.to_string())),
    }
}

#[cfg(test)]
#[path = "http_conn_tests.rs"]
mod tests;
