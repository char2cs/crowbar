use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::UnixListener;

use super::*;

fn socket(tag: &str) -> std::path::PathBuf {
    std::env::temp_dir().join(format!("cb-lget-{tag}-{}.sock", std::process::id()))
}

async fn serve_once(listener: UnixListener, reply: &'static str) -> String {
    let (mut stream, _) = listener.accept().await.unwrap();
    let mut buf = vec![0u8; 4096];
    let read = stream.read(&mut buf).await.unwrap();
    stream.write_all(reply.as_bytes()).await.unwrap();
    String::from_utf8_lossy(&buf[..read]).into_owned()
}

#[tokio::test]
async fn it_gets_the_target_and_returns_the_buffered_answer() {
    let path = socket("ok");
    let listener = UnixListener::bind(&path).unwrap();
    let daemon = tokio::spawn(serve_once(
        listener,
        "HTTP/1.1 200 OK\r\ncontent-length: 2\r\nconnection: close\r\n\r\nhi",
    ));
    let stream = UnixStream::connect(&path).await.unwrap();
    let response = local_get(stream, "/v0/health?x=1", 1024).await.unwrap();
    assert_eq!(response.status(), 200);
    assert_eq!(response.body(), b"hi");
    let seen = daemon.await.unwrap();
    assert!(seen.starts_with("GET /v0/health?x=1 HTTP/1.1"), "{seen}");
    std::fs::remove_file(&path).unwrap();
}

#[tokio::test]
async fn an_answer_over_the_limit_is_refused() {
    let path = socket("big");
    let listener = UnixListener::bind(&path).unwrap();
    tokio::spawn(serve_once(
        listener,
        "HTTP/1.1 200 OK\r\ncontent-length: 10\r\nconnection: close\r\n\r\n0123456789",
    ));
    let stream = UnixStream::connect(&path).await.unwrap();
    let error = local_get(stream, "/v0/x", 4).await.unwrap_err();
    assert!(matches!(error, Error::TooLarge), "{error}");
    std::fs::remove_file(&path).unwrap();
}
