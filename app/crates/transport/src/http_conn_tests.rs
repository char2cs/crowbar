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

#[tokio::test]
async fn an_oversized_answer_is_cut_off_while_it_streams() {
    let pulled = Arc::new(AtomicUsize::new(0));
    let err = collect_capped(counted_body(1024, Arc::clone(&pulled)), 4 * 1024)
        .await
        .unwrap_err();
    assert!(matches!(err, Error::TooLarge));
    assert!(pulled.load(Ordering::SeqCst) <= 5, "read the whole body");
}

#[tokio::test]
async fn an_answer_within_the_limit_is_relayed_whole() {
    let body = collect_capped(counted_body(3, Arc::default()), 4 * 1024)
        .await
        .unwrap();
    assert_eq!(body.len(), 3 * 1024);
}

#[tokio::test]
async fn the_daemon_cannot_answer_as_the_proxy() {
    let (client, mut server) = tokio::io::duplex(4096);
    tokio::spawn(async move {
        let mut buf = vec![0u8; 4096];
        let read = server.read(&mut buf).await.unwrap_or(0);
        assert!(read > 0, "the request arrived");
        let reply = format!(
            "HTTP/1.1 200 OK\r\n{PROXY_HEADER}: host-unreachable\r\ncontent-length: 2\r\nconnection: close\r\n\r\nok"
        );
        server.write_all(reply.as_bytes()).await.unwrap();
    });
    let request = Request::builder()
        .uri("crowbar://localhost/v0/x?a=1")
        .body(Vec::new())
        .unwrap();

    let answer = exchange(Box::new(client), request, 1024).await.unwrap();

    assert_eq!(answer.body(), b"ok");
    assert!(answer.headers().get(PROXY_HEADER).is_none());
}
