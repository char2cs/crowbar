use std::path::PathBuf;
use std::sync::atomic::{AtomicU32, Ordering};

use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::UnixListener;

use super::*;
use crate::response::marker_of;

fn socket(tag: &str) -> PathBuf {
    let dir = std::env::temp_dir().join(format!("cb-l-{tag}-{}", std::process::id()));
    std::fs::create_dir_all(&dir).unwrap();
    dir.join("d.sock")
}

fn request(method: Method) -> Request<Vec<u8>> {
    Request::builder()
        .method(method)
        .uri("crowbar://localhost/v0/health")
        .body(Vec::new())
        .unwrap()
}

#[test]
fn retries_poll_densely_then_back_off_to_a_cap() {
    assert_eq!(connect_retry_delay(1), Duration::from_millis(25));
    assert_eq!(connect_retry_delay(40), Duration::from_millis(25));
    assert_eq!(connect_retry_delay(41), Duration::from_millis(100));
    assert_eq!(connect_retry_delay(42), Duration::from_millis(200));
    assert_eq!(connect_retry_delay(46), Duration::from_secs(1));
    assert_eq!(connect_retry_delay(1000), Duration::from_secs(1));
}

#[tokio::test]
async fn a_read_retries_until_the_attempts_are_spent_then_says_the_daemon_is_unavailable() {
    let waits = AtomicU32::new(0);
    let path = socket("none");

    let answer = forward_local(&path, request(Method::GET), |_| {
        waits.fetch_add(1, Ordering::SeqCst);
        std::future::ready(())
    })
    .await;

    assert_eq!(answer.status(), 503);
    assert_eq!(marker_of(&answer), Some(DAEMON_UNAVAILABLE));
    assert_eq!(waits.load(Ordering::SeqCst), CONNECT_ATTEMPTS - 1);
}

#[tokio::test]
async fn a_mutation_is_never_replayed() {
    let waits = AtomicU32::new(0);
    let path = socket("post");

    let answer = forward_local(&path, request(Method::POST), |_| {
        waits.fetch_add(1, Ordering::SeqCst);
        std::future::ready(())
    })
    .await;

    assert_eq!(answer.status(), 503);
    assert_eq!(waits.load(Ordering::SeqCst), 0);
}

fn serve_ok_once(path: &std::path::Path) {
    let bound = UnixListener::bind(path).unwrap();
    tokio::spawn(answer_ok(bound));
}

async fn answer_ok(bound: UnixListener) {
    let (mut stream, _) = bound.accept().await.unwrap();
    let mut buf = [0u8; 1024];
    let read = stream.read(&mut buf).await.unwrap();
    assert!(read > 0, "the request arrived");
    let reply = b"HTTP/1.1 200 OK\r\ncontent-length: 2\r\nconnection: close\r\n\r\nok";
    stream.write_all(reply).await.unwrap();
}

fn bind_on_third_attempt(path: &std::path::Path, attempt: u32) {
    if attempt == 3 {
        serve_ok_once(path);
    }
}

#[tokio::test]
async fn a_daemon_that_binds_late_is_reached_by_a_retried_read() {
    let path = socket("late");
    let listener_path = path.clone();
    let attempts = AtomicU32::new(0);

    let answer = forward_local(&path, request(Method::GET), |attempt| {
        attempts.store(attempt, Ordering::SeqCst);
        bind_on_third_attempt(&listener_path, attempt);
        std::future::ready(())
    })
    .await;

    assert_eq!(answer.status(), 200);
    assert_eq!(answer.body(), b"ok");
    assert_eq!(attempts.load(Ordering::SeqCst), 3);
}
