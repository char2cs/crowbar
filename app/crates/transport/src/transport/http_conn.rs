use bytes::Bytes;
use http::{HeaderMap, HeaderValue, Request, Response, header};
use http_body_util::{BodyExt, Full, LengthLimitError, Limited};
use hyper::client::conn::http1::SendRequest;
use hyper_util::rt::TokioIo;

use crate::error::Error;
use crate::response::PROXY_HEADER;
use crate::transport::async_stream::BoxedStream;

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
    tokio::spawn(drive(connection));
    Ok(sender)
}

async fn drive<I>(connection: hyper::client::conn::http1::Connection<I, Full<Bytes>>)
where
    I: hyper::rt::Read + hyper::rt::Write + Unpin,
{
    if let Err(e) = connection.await {
        tracing::debug!("daemon connection ended: {e}");
    }
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

fn copy_relayed_headers(
    from: &HeaderMap,
    to: &mut HeaderMap,
) {
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
    let mut relayed = Response::new(collect_capped(body, body_limit).await?);
    *relayed.status_mut() = parts.status;
    copy_relayed_headers(&parts.headers, relayed.headers_mut());
    Ok(relayed)
}

pub(crate) async fn exchange(
    stream: BoxedStream,
    request: Request<Vec<u8>>,
    body_limit: usize,
) -> Result<Response<Vec<u8>>, Error> {
    let mut channel = open_channel(stream).await?;
    send(&mut channel, request, body_limit).await
}

pub(crate) async fn collect_capped<B>(
    body: B,
    limit: usize,
) -> Result<Vec<u8>, Error>
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
mod tests {
    use std::convert::Infallible;
    use std::sync::Arc;
    use std::sync::atomic::{AtomicUsize, Ordering};

    use futures_util::stream;
    use http_body_util::StreamBody;
    use hyper::body::Frame;
    use tokio::io::{AsyncReadExt, AsyncWriteExt};

    use super::*;

    fn counted_body(
        chunks: usize,
        pulled: Arc<AtomicUsize>,
    ) -> impl hyper::body::Body<Data = Bytes, Error = Infallible> {
        let frames = (0..chunks).map(move |_| {
            pulled.fetch_add(1, Ordering::SeqCst);
            Ok(Frame::data(Bytes::from_static(&[0u8; 1024])))
        });
        StreamBody::new(stream::iter(frames))
    }

    const CHUNK: usize = 1024;
    const LIMIT: usize = 4 * CHUNK;

    /// (chunks the daemon would send, the relayed length or whether it was cut off as too large)
    type Relayed = Result<usize, bool>;

    const CASES: [(usize, Relayed); 2] = [(1024, Err(true)), (3, Ok(3 * CHUNK))];

    async fn assert_relayed_or_cut_off() {
        for (chunks, expected) in CASES {
            let pulled = Arc::new(AtomicUsize::new(0));
            let result = collect_capped(counted_body(chunks, Arc::clone(&pulled)), LIMIT).await;
            let observed = result
                .map(|body| body.len())
                .map_err(|e| matches!(e, Error::TooLarge));
            assert_eq!(observed, expected, "{chunks} chunks");
            let read_at_most = chunks.min(LIMIT / CHUNK + 1);
            assert!(
                pulled.load(Ordering::SeqCst) <= read_at_most,
                "read the whole body"
            );
        }
    }

    #[tokio::test]
    async fn an_answer_is_relayed_whole_within_the_limit_and_cut_off_while_it_streams_beyond() {
        assert_relayed_or_cut_off().await;
    }

    async fn reply_once(
        mut server: tokio::io::DuplexStream,
        reply: Vec<u8>,
    ) {
        let mut buf = vec![0u8; 4096];
        let read = server.read(&mut buf).await.unwrap_or(0);
        assert!(read > 0, "the request arrived");
        server.write_all(&reply).await.unwrap();
    }

    #[tokio::test]
    async fn the_daemon_cannot_answer_as_the_proxy() {
        let (client, server) = tokio::io::duplex(4096);
        let reply = format!(
            "HTTP/1.1 200 OK\r\n{PROXY_HEADER}: host-unreachable\r\ncontent-length: 2\r\nconnection: close\r\n\r\nok"
        );
        tokio::spawn(reply_once(server, reply.into_bytes()));
        let request = Request::builder()
            .uri("crowbar://localhost/v0/x?a=1")
            .body(Vec::new())
            .unwrap();

        let answer = exchange(Box::new(client), request, 1024).await.unwrap();

        assert_eq!(answer.body(), b"ok");
        assert!(answer.headers().get(PROXY_HEADER).is_none());
    }

    async fn echo_head(request: Request<Vec<u8>>) -> String {
        let (client, mut server) = tokio::io::duplex(8192);
        let seen = tokio::spawn(async move {
            let mut buf = vec![0u8; 8192];
            let read = server.read(&mut buf).await.unwrap_or(0);
            server
                .write_all(b"HTTP/1.1 200 OK\r\ncontent-length: 0\r\nconnection: close\r\n\r\n")
                .await
                .unwrap();
            String::from_utf8_lossy(buf.get(..read).unwrap_or_default()).to_lowercase()
        });
        exchange(Box::new(client), request, 1024).await.unwrap();
        seen.await.unwrap()
    }

    #[tokio::test]
    async fn the_daemon_sees_a_constant_host_and_none_of_the_panes_hop_by_hop_or_credential_headers()
     {
        let mut request = Request::builder()
            .uri("crowbar://localhost/v0/x")
            .header("x-pane", "p1")
            .header("host", "evil.example")
            .header("authorization", "Bearer pane-guess")
            .header("connection", "upgrade")
            .header("upgrade", "websocket")
            .header("transfer-encoding", "chunked")
            .header("content-length", "999")
            .body(Vec::new())
            .unwrap();
        strip_pane_headers(request.headers_mut());
        let head = echo_head(request).await;
        assert!(head.contains("x-pane: p1"), "{head}");
        assert!(head.contains("host: localhost"), "{head}");
        assert_nothing_leaked(&head);
    }

    fn assert_nothing_leaked(head: &str) {
        for banned in ["evil.example", "pane-guess", "upgrade", "chunked", "999"] {
            assert!(
                !head.contains(banned),
                "{banned} reached the daemon: {head}"
            );
        }
    }

    fn assert_hop_headers_dropped(answer: &Response<Vec<u8>>) {
        for hop in ["transfer-encoding", "keep-alive", "connection"] {
            assert!(answer.headers().get(hop).is_none(), "{hop}");
        }
    }

    #[tokio::test]
    async fn hop_by_hop_answer_headers_are_not_relayed_with_a_buffered_body() {
        let (client, server) = tokio::io::duplex(4096);
        let reply = b"HTTP/1.1 200 OK\r\ntransfer-encoding: chunked\r\nkeep-alive: timeout=5\r\nx-kept: 1\r\nconnection: close\r\n\r\n2\r\nok\r\n0\r\n\r\n";
        tokio::spawn(reply_once(server, reply.to_vec()));
        let request = Request::builder().uri("/v0/x").body(Vec::new()).unwrap();
        let answer = exchange(Box::new(client), request, 1024).await.unwrap();
        assert_eq!(answer.body(), b"ok");
        assert_eq!(answer.headers().get("x-kept").unwrap(), "1");
        assert_hop_headers_dropped(&answer);
    }
}
