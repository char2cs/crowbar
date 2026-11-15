use bytes::Bytes;
use http_body_util::{BodyExt, Full, Limited};
use hyper::Request as HyperRequest;
use hyper_util::rt::TokioIo;
use tauri::http;
use tokio::io::{AsyncRead, AsyncWrite};

use super::PROXY_HEADER;

pub(crate) type BoxError = Box<dyn std::error::Error + Send + Sync>;

pub(crate) type Channel = hyper::client::conn::http1::SendRequest<Full<Bytes>>;

const FALLBACK_HOST: &str = "localhost";

pub(crate) async fn proxy<S>(
    stream: S,
    request: http::Request<Vec<u8>>,
    body_limit: usize,
) -> Result<http::Response<Vec<u8>>, BoxError>
where
    S: AsyncRead + AsyncWrite + Unpin + Send + 'static,
{
    let mut channel = open_channel(stream).await?;
    send(&mut channel, request, body_limit).await
}

pub(crate) async fn open_channel<S>(stream: S) -> Result<Channel, BoxError>
where
    S: AsyncRead + AsyncWrite + Unpin + Send + 'static,
{
    let io = TokioIo::new(stream);
    let (sender, conn) = hyper::client::conn::http1::handshake(io).await?;
    tokio::spawn(async move { log_ended(conn.await) });
    Ok(sender)
}

fn log_ended(result: hyper::Result<()>) {
    if let Err(error) = result {
        log::debug!("api proxy: connection ended: {error}");
    }
}

pub(crate) async fn send(
    channel: &mut Channel,
    request: http::Request<Vec<u8>>,
    body_limit: usize,
) -> Result<http::Response<Vec<u8>>, BoxError> {
    channel.ready().await?;
    let upstream = upstream_request(request)?;
    let (parts, body) = channel.send_request(upstream).await?.into_parts();
    let collected = collect_capped(body, body_limit).await?;
    Ok(relayed_response(&parts, collected))
}

fn upstream_request(
    request: http::Request<Vec<u8>>
) -> Result<HyperRequest<Full<Bytes>>, BoxError> {
    let path_and_query = request.uri().path_and_query().map_or_else(
        || request.uri().path().to_owned(),
        |pq| pq.as_str().to_owned(),
    );
    let (parts, body) = request.into_parts();
    let mut upstream = HyperRequest::builder()
        .method(parts.method)
        .uri(path_and_query)
        .body(Full::new(Bytes::from(body)))?;
    let headers = upstream.headers_mut();
    for (name, value) in &parts.headers {
        headers.insert(name, value.clone());
    }
    headers
        .entry(http::header::HOST)
        .or_insert_with(|| http::HeaderValue::from_static(FALLBACK_HOST));
    Ok(upstream)
}

fn relayed_response(
    parts: &http::response::Parts,
    body: Vec<u8>,
) -> http::Response<Vec<u8>> {
    let mut relayed_resp = http::Response::new(body);
    *relayed_resp.status_mut() = parts.status;
    let relayed = parts
        .headers
        .iter()
        .filter(|(name, _)| *name != PROXY_HEADER);
    for (name, value) in relayed {
        relayed_resp.headers_mut().insert(name, value.clone());
    }
    relayed_resp
}

pub(crate) async fn collect_capped<B>(
    body: B,
    limit: usize,
) -> Result<Vec<u8>, BoxError>
where
    B: hyper::body::Body,
    B::Error: Into<BoxError>,
{
    let collected = Limited::new(body, limit).collect().await?;
    Ok(collected.to_bytes().to_vec())
}

#[cfg(test)]
mod tests {
    use std::convert::Infallible;
    use std::sync::atomic::{AtomicUsize, Ordering};
    use std::sync::Arc;

    use futures_util::StreamExt;
    use http_body_util::{LengthLimitError, StreamBody};
    use hyper::body::Frame;

    use super::*;
    use crate::test_support::proxy_fakes::{duplex_peer, header, response};

    const KIB: usize = 1024;

    fn counted_body(
        chunks: usize,
        pulled: Arc<AtomicUsize>,
    ) -> impl hyper::body::Body<Data = Bytes, Error = Infallible> {
        StreamBody::new(futures_util::stream::iter(0..chunks).map(move |_| {
            pulled.fetch_add(1, Ordering::SeqCst);
            Ok(Frame::data(Bytes::from(vec![0u8; KIB])))
        }))
    }

    fn assert_cut_off(
        error: &(dyn std::error::Error + Send + Sync + 'static),
        pulled: &AtomicUsize,
    ) {
        assert!(error.is::<LengthLimitError>(), "{error}");
        assert!(pulled.load(Ordering::SeqCst) <= 5, "read past the limit");
    }

    #[tokio::test]
    async fn a_request_is_relayed_by_path_with_a_default_host_and_without_the_hosts_marker() {
        let spoofed = format!(
            "HTTP/1.1 201 Created\r\n{PROXY_HEADER}: spoofed\r\nx-kept: yes\r\ncontent-length: 2\r\nconnection: close\r\n\r\nok"
        );
        let (stream, reported) = duplex_peer(spoofed.into_bytes());
        let request = http::Request::builder()
            .uri("crowbar://host-1/v0/projects?limit=1")
            .body(Vec::new())
            .unwrap();

        let resp = proxy(stream, request, usize::MAX).await.unwrap();

        let head = reported.await.unwrap();
        assert!(
            head.starts_with("GET /v0/projects?limit=1 HTTP/1.1\r\n"),
            "{head}"
        );
        assert_eq!(header(&head, "host"), Some(FALLBACK_HOST));
        assert_eq!(resp.status(), 201);
        assert_eq!(resp.body().as_slice(), b"ok");
        assert!(resp.headers().get(PROXY_HEADER).is_none(), "marker relayed");
        assert_eq!(resp.headers().get("x-kept").unwrap(), "yes");
    }

    #[tokio::test]
    async fn a_body_is_cut_off_while_it_streams_once_it_passes_the_limit() {
        a_body_is_cut_off_while_it_streams_once_run().await;
    }

    type Case = (usize, usize, Option<usize>);

    async fn a_body_is_cut_off_while_it_streams_once_run() {
        const CASES: &[Case] = &[(1024, 4 * KIB, None), (3, 4 * KIB, Some(3 * KIB))];
        for (chunks, limit, whole) in CASES {
            let pulled = Arc::new(AtomicUsize::new(0));

            let collected =
                collect_capped(counted_body(*chunks, Arc::clone(&pulled)), *limit).await;

            match (collected, whole) {
                (Ok(body), Some(len)) => assert_eq!(body.len(), *len, "{chunks} chunks"),
                (Err(error), None) => assert_cut_off(&*error, &pulled),
                (other, _) => panic!("{chunks} chunks: unexpected {:?}", other.map(|b| b.len())),
            }
        }
    }

    #[tokio::test]
    async fn a_daemon_that_hangs_up_without_answering_is_an_error() {
        let (stream, _reported) = duplex_peer(response("200 OK", "ok")[..4].to_vec());
        let request = http::Request::builder()
            .uri("crowbar://host-1/v0/x")
            .body(Vec::new())
            .unwrap();

        let resp = proxy(stream, request, usize::MAX).await;

        assert!(resp.is_err(), "a truncated answer is not a response");
    }
}
