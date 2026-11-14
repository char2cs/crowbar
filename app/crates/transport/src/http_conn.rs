use bytes::Bytes;
use http::{HeaderMap, HeaderValue, Request, Response, header};
use http_body_util::{BodyExt, Full, LengthLimitError, Limited};
use hyper::client::conn::http1::SendRequest;
use hyper_util::rt::TokioIo;

use crate::async_stream::BoxedStream;
use crate::error::Error;
use crate::response::PROXY_HEADER;

const UPSTREAM_HOST: &str = "localhost";

const HOP_BY_HOP: [&str; 7] = [
    "connection",
    "keep-alive",
    "proxy-connection",
    "te",
    "trailer",
    "transfer-encoding",
    "upgrade",
];

const PANE_CONTROLLED: [&str; 4] = [
    "host",
    "content-length",
    "authorization",
    "proxy-authorization",
];

pub(crate) type Channel = SendRequest<Full<Bytes>>;

pub(crate) async fn open_channel(stream: BoxedStream) -> Result<Channel, Error> {
    let (sender, connection) = hyper::client::conn::http1::handshake(TokioIo::new(stream))
        .await
        .map_err(Error::exchange)?;
    tokio::spawn(async move {
        if let Err(e) = connection.await {
            tracing::debug!("daemon connection ended: {e}");
        }
    });
    Ok(sender)
}

pub(crate) fn strip_pane_headers(headers: &mut HeaderMap) {
    for name in HOP_BY_HOP.into_iter().chain(PANE_CONTROLLED) {
        headers.remove(name);
    }
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
        headers.insert(header::HOST, HeaderValue::from_static(UPSTREAM_HOST));
    }
    upstream
        .body(Full::new(Bytes::from(body)))
        .map_err(Error::bad_request)
}

fn copy_relayed_headers(from: &HeaderMap, to: &mut HeaderMap) {
    let relayed = from
        .iter()
        .filter(|(name, _)| name.as_str() != PROXY_HEADER && !HOP_BY_HOP.contains(&name.as_str()));
    for (name, value) in relayed {
        to.append(name.clone(), value.clone());
    }
}

pub(crate) async fn send(
    channel: &mut Channel,
    request: Request<Vec<u8>>,
    body_limit: usize,
) -> Result<Response<Vec<u8>>, Error> {
    channel.ready().await.map_err(Error::exchange)?;
    let answer = channel
        .send_request(upstream_request(request)?)
        .await
        .map_err(Error::exchange)?;
    let (parts, body) = answer.into_parts();
    let mut out = Response::new(collect_capped(body, body_limit).await?);
    *out.status_mut() = parts.status;
    copy_relayed_headers(&parts.headers, out.headers_mut());
    Ok(out)
}

pub(crate) async fn exchange(
    stream: BoxedStream,
    request: Request<Vec<u8>>,
    body_limit: usize,
) -> Result<Response<Vec<u8>>, Error> {
    let mut channel = open_channel(stream).await?;
    send(&mut channel, request, body_limit).await
}

pub(crate) async fn collect_capped<B>(body: B, limit: usize) -> Result<Vec<u8>, Error>
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
