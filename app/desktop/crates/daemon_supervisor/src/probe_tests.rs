use std::path::PathBuf;

use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::{UnixListener, UnixStream};

use super::*;
use crate::failure_tracker::FailureTracker;

const OK: &str = include_str!("../../../../../api/internal/core/bootgate/testdata/health-ok.json");
const MIGRATING: &str =
    include_str!("../../../../../api/internal/core/bootgate/testdata/health-migrating.json");
const HALTED: &str =
    include_str!("../../../../../api/internal/core/bootgate/testdata/health-halted.json");
const THRESHOLD: u32 = 6;

fn socket(tag: &str) -> PathBuf {
    std::env::temp_dir().join(format!("cb-probe-{tag}-{}.sock", std::process::id()))
}

fn serve(path: &Path, health: &'static str, other: u16) -> tokio::task::JoinHandle<()> {
    let listener = UnixListener::bind(path).unwrap();
    tokio::spawn(serve_forever(listener, health, other))
}

async fn serve_forever(listener: UnixListener, health: &'static str, other: u16) {
    loop {
        let (stream, _) = listener.accept().await.unwrap();
        answer_one(stream, health, other).await;
    }
}

async fn answer_one(mut stream: UnixStream, health: &'static str, other: u16) {
    let mut buf = vec![0u8; 4096];
    let read = stream.read(&mut buf).await.unwrap_or(0);
    if read == 0 {
        return;
    }
    let is_health = buf.get(..read).is_some_and(|head| head.starts_with(b"GET /v0/health "));
    let (status, body) = if is_health { (200, health) } else { (other, "{}") };
    let reply = format!(
        "HTTP/1.1 {status} X\r\ncontent-length: {}\r\nconnection: close\r\n\r\n{body}",
        body.len()
    );
    stream.write_all(reply.as_bytes()).await.ok();
}

async fn watched(tag: &str, health: &'static str, other: u16) -> (Vec<Probe>, bool) {
    let path = socket(tag);
    let daemon = serve(&path, health, other);
    let mut tracker = FailureTracker::new(THRESHOLD);
    let (mut outcomes, mut tripped) = (Vec::new(), false);
    for _ in 0..THRESHOLD * 2 {
        let outcome = probe_ready(&path).await;
        tripped |= tracker.observe(outcome);
        outcomes.push(outcome);
    }
    daemon.abort();
    std::fs::remove_file(&path).unwrap();
    (outcomes, tripped)
}

#[tokio::test]
async fn a_migrating_or_halted_daemon_is_held_and_never_killed() {
    for (tag, health) in [("migrating", MIGRATING), ("halted", HALTED)] {
        let (outcomes, tripped) = watched(tag, health, 503).await;
        assert!(
            outcomes.iter().all(|o| *o == Probe::Held),
            "{tag}: {outcomes:?}"
        );
        assert!(!tripped, "{tag}");
    }
}

#[tokio::test]
async fn a_serving_daemon_whose_deep_probe_fails_is_still_indicted() {
    let (outcomes, tripped) = watched("wedged", OK, 503).await;
    assert!(
        outcomes.iter().all(|o| *o == Probe::Unserved),
        "{outcomes:?}"
    );
    assert!(tripped);
}

#[tokio::test]
async fn a_serving_daemon_that_answers_is_healthy() {
    let (outcomes, tripped) = watched("healthy", OK, 200).await;
    assert!(
        outcomes.iter().all(|o| *o == Probe::Healthy),
        "{outcomes:?}"
    );
    assert!(!tripped);
}

#[tokio::test]
async fn a_missing_socket_is_unserved_and_descriptor_errors_are_recognised() {
    assert_eq!(probe_ready(&socket("absent")).await, Probe::Unserved);
    assert!(is_descriptor_exhaustion(
        &std::io::Error::from_raw_os_error(Errno::MFILE.raw_os_error())
    ));
    assert!(is_descriptor_exhaustion(
        &std::io::Error::from_raw_os_error(Errno::NFILE.raw_os_error())
    ));
    assert!(!is_descriptor_exhaustion(
        &std::io::Error::from_raw_os_error(Errno::CONNREFUSED.raw_os_error())
    ));
}
