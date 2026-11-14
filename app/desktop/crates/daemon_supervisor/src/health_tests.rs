use std::path::PathBuf;

use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::{UnixListener, UnixStream};

use super::*;

const OK: &str = include_str!("../../../../../api/internal/core/bootgate/testdata/health-ok.json");
const MIGRATING: &str =
    include_str!("../../../../../api/internal/core/bootgate/testdata/health-migrating.json");
const HALTED: &str =
    include_str!("../../../../../api/internal/core/bootgate/testdata/health-halted.json");
const HALT_MESSAGE: &str =
    "this home is at layout 9 but this Crowbar only knows layout 0: run a newer Crowbar";
const POLL: Duration = Duration::from_millis(1);
const PROBE: Duration = Duration::from_secs(2);

fn socket(tag: &str) -> PathBuf {
    std::env::temp_dir().join(format!("cb-health-{tag}-{}.sock", std::process::id()))
}

fn serve(path: &Path, health: Vec<&'static str>) -> tokio::task::JoinHandle<()> {
    let listener = UnixListener::bind(path).unwrap();
    tokio::spawn(serve_forever(listener, health))
}

async fn serve_forever(listener: UnixListener, health: Vec<&'static str>) {
    let mut asked = 0;
    loop {
        let (stream, _) = listener.accept().await.unwrap();
        let last = health.len().saturating_sub(1);
        answer_one(stream, health.get(asked.min(last)).copied().unwrap_or("{}")).await;
        asked += 1;
    }
}

async fn answer_one(mut stream: UnixStream, body: &str) {
    let mut buf = vec![0u8; 4096];
    let read = stream.read(&mut buf).await.unwrap();
    assert!(buf.get(..read).is_some_and(|head| head.starts_with(b"GET /v0/health ")));
    let reply = format!(
        "HTTP/1.1 200 OK\r\ncontent-length: {}\r\nconnection: close\r\n\r\n{body}",
        body.len()
    );
    stream.write_all(reply.as_bytes()).await.unwrap();
}

#[test]
fn the_three_known_states_map_to_their_health() {
    assert_eq!(
        parse_health(OK.as_bytes()),
        Health::Ready { pid: Some(4242) }
    );
    assert_eq!(parse_health(MIGRATING.as_bytes()), Health::Migrating);
    assert_eq!(
        parse_health(HALTED.as_bytes()),
        Health::Halted {
            message: HALT_MESSAGE.into()
        }
    );
}

#[test]
fn a_halt_without_a_message_still_halts() {
    let body = br#"{"success":true,"data":{"status":"halted"}}"#;
    assert!(matches!(parse_health(body), Health::Halted { .. }));
}

#[test]
fn a_daemon_that_names_no_state_or_an_unknown_one_is_ready() {
    assert_eq!(
        parse_health(br#"{"data":{"pid":7}}"#),
        Health::Ready { pid: Some(7) }
    );
    assert_eq!(
        parse_health(br#"{"data":{"status":"resting","pid":7}}"#),
        Health::Ready { pid: Some(7) }
    );
}

#[test]
fn an_unreadable_body_or_pid_zero_is_ready_without_a_pid() {
    for body in [
        &b"not json"[..],
        br#"{"data":{"status":"ok"}}"#,
        br#"{"data":{"pid":0}}"#,
    ] {
        assert_eq!(parse_health(body), Health::Ready { pid: None });
    }
}

#[tokio::test]
async fn a_migrating_daemon_is_waited_for_without_spending_the_budget() {
    let path = socket("migrating");
    let daemon = serve(&path, vec![MIGRATING, MIGRATING, MIGRATING, OK]);
    let pid = wait_for_health(&path, 1, POLL, PROBE).await.unwrap();
    daemon.abort();
    std::fs::remove_file(&path).unwrap();
    assert_eq!(pid, Some(4242));
}

#[tokio::test]
async fn a_halted_daemon_ends_the_wait_with_its_reason() {
    let path = socket("halted");
    let daemon = serve(&path, vec![MIGRATING, HALTED]);
    let outcome = wait_for_health(&path, 30, POLL, PROBE).await;
    daemon.abort();
    std::fs::remove_file(&path).unwrap();
    assert!(
        matches!(outcome, Err(Error::Halted(ref m)) if m == HALT_MESSAGE),
        "{outcome:?}"
    );
}

#[tokio::test]
async fn a_daemon_that_never_answers_runs_the_budget_down() {
    let outcome = wait_for_health(&socket("absent"), 2, POLL, PROBE).await;
    assert!(matches!(outcome, Err(Error::GaveUp(2))), "{outcome:?}");
}
