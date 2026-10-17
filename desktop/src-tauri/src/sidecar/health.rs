//! What `/v0/health` says about the daemon: serving, still preparing its home,
//! or halted. A daemon that is preparing or halted is alive on purpose; it must
//! never be mistaken for a wedge.

use std::fmt;
use std::time::Duration;

use http_body_util::BodyExt;
use serde_json::Value;
use tauri::{AppHandle, Emitter, Runtime};

use super::http_get;

/// Per-attempt budget for the startup health probe. A booting daemon answers
/// /v0/health immediately or not at all, so this only has to outlast a loaded
/// machine — never a slow request.
const HEALTH_PROBE_TIMEOUT: Duration = Duration::from_secs(3);

#[derive(Debug, PartialEq, Eq)]
pub enum Health {
    Ready { pid: Option<i32> },
    Migrating,
    Halted { message: String },
}

/// Reads the /v0/health envelope. The halt text is `data.detail.message`. The
/// self-reported pid is the only safe one: `CommandChild::pid()` deadlocks on a
/// live daemon.
pub fn parse_health(body: &[u8]) -> Health {
    let data = serde_json::from_slice::<Value>(body)
        .ok()
        .and_then(|v| v.get("data").cloned());
    let field = |name: &str| data.as_ref().and_then(|d| d.get(name));
    match field("status").and_then(Value::as_str) {
        Some("migrating") => Health::Migrating,
        Some("halted") => Health::Halted {
            message: field("detail")
                .and_then(|d| d.get("message"))
                .and_then(Value::as_str)
                .unwrap_or("the daemon halted without saying why")
                .to_string(),
        },
        // An absent status is an older daemon and an unknown one a newer daemon;
        // both are serving, so only the deep probe may judge them.
        _ => Health::Ready {
            pid: field("pid")
                .and_then(Value::as_i64)
                .and_then(|pid| i32::try_from(pid).ok())
                .filter(|pid| *pid > 0),
        },
    }
}

/// Why the wait for a daemon to serve ended without it serving.
#[derive(Debug)]
pub enum Unready {
    Halted(String),
    GaveUp(u32),
}

impl fmt::Display for Unready {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Halted(message) => write!(f, "the daemon halted: {message}"),
            Self::GaveUp(attempts) => {
                write!(f, "daemon did not become healthy in {attempts} attempts")
            }
        }
    }
}

impl std::error::Error for Unready {}

/// Tells the window why the daemon will not come up. Nothing restarts a halted
/// daemon, so this is the user's only explanation.
pub fn announce_halt<R: Runtime>(app: &AppHandle<R>, why: &Unready) {
    if let Unready::Halted(message) = why {
        log::error!("crowbar daemon halted: {message}");
        let _ = app.emit("daemon:halted", message);
    }
}

/// Probes /v0/health; on success reports what the daemon says about itself.
pub async fn check_health(
    socket: &std::path::PathBuf,
) -> Result<Health, Box<dyn std::error::Error + Send + Sync>> {
    let resp = http_get(socket, "/v0/health").await?;
    if !resp.status().is_success() {
        return Err(format!("health check returned {}", resp.status()).into());
    }
    let body = resp.into_body().collect().await?.to_bytes();
    Ok(parse_health(&body))
}

/// Polls until the daemon serves, returning its self-reported pid (None for a
/// daemon predating the pid field), or until it halts. A migrating daemon is
/// alive and busy, so it is waited for without spending `attempts`; only a
/// daemon that cannot be reached, or answers wrongly, does.
pub async fn wait_for_health(
    socket: &std::path::PathBuf,
    attempts: u32,
) -> Result<Option<i32>, Unready> {
    let mut failed = 0;
    loop {
        tokio::time::sleep(Duration::from_millis(200)).await;

        // Budget every attempt. A daemon that accepts the connection but never
        // serves /v0/health would otherwise park this loop forever — and with it
        // `spawn`, which is what records `daemon_pid`. The watchdog would then trip,
        // find no pid, and fall back to `CommandChild::kill()`: the one call that
        // deadlocks on a live daemon (see `SidecarHandle`). The supervisor would
        // never run again, so the leak is not the descriptor, it is the recovery.
        match tokio::time::timeout(HEALTH_PROBE_TIMEOUT, check_health(socket)).await {
            Ok(Ok(Health::Ready { pid })) => return Ok(pid),
            Ok(Ok(Health::Halted { message })) => return Err(Unready::Halted(message)),
            Ok(Ok(Health::Migrating)) => continue,
            _ => failed += 1,
        }
        if failed >= attempts {
            return Err(Unready::GaveUp(attempts));
        }
    }
}

#[cfg(test)]
mod tests {
    use super::{parse_health, wait_for_health, Health, Unready};
    use std::path::{Path, PathBuf};
    use tokio::io::{AsyncReadExt, AsyncWriteExt};
    use tokio::net::UnixListener;

    const OK: &str = r#"{"success":true,"data":{"status":"ok","version":"0.1.0","pid":42}}"#;
    const MIGRATING: &str = r#"{"success":true,"data":{"status":"migrating","version":"0.1.0","pid":42,"detail":{"phase":"pre-open"}}}"#;
    const HALTED: &str = r#"{"success":true,"data":{"status":"halted","version":"0.1.0","pid":42,"detail":{"reason":"layout_newer","message":"this home is at layout 9"}}}"#;

    #[test]
    fn the_three_known_states_map_to_their_health() {
        assert_eq!(parse_health(OK.as_bytes()), Health::Ready { pid: Some(42) });
        assert_eq!(parse_health(MIGRATING.as_bytes()), Health::Migrating);
        assert_eq!(
            parse_health(HALTED.as_bytes()),
            Health::Halted {
                message: "this home is at layout 9".into()
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
            parse_health(br#"{"success":true,"data":{"pid":7}}"#),
            Health::Ready { pid: Some(7) },
            "older daemons predate the status field"
        );
        assert_eq!(
            parse_health(br#"{"success":true,"data":{"status":"resting","pid":7}}"#),
            Health::Ready { pid: Some(7) }
        );
    }

    #[test]
    fn an_unreadable_body_is_ready_without_a_pid() {
        for body in [
            &b"not json"[..],
            br#"{"success":true,"data":{"status":"ok"}}"#,
        ] {
            assert_eq!(parse_health(body), Health::Ready { pid: None });
        }
        assert_eq!(
            parse_health(br#"{"success":true,"data":{"pid":0}}"#),
            Health::Ready { pid: None },
            "pid 0 would signal the whole process group, so it is never accepted"
        );
    }

    fn socket(tag: &str) -> PathBuf {
        std::env::temp_dir().join(format!("cb-health-{tag}-{}.sock", std::process::id()))
    }

    /// Binds `path` and answers its n-th connection with `bodies[n]`; the handle
    /// finishes once every body has been served.
    fn serve_script(path: &Path, bodies: Vec<&'static str>) -> tokio::task::JoinHandle<()> {
        let listener = UnixListener::bind(path).expect("bind test daemon socket");
        tokio::spawn(async move {
            for body in bodies {
                let (mut stream, _) = listener.accept().await.expect("accept");
                let mut buf = vec![0u8; 4096];
                let mut read = 0;
                while !buf[..read].windows(4).any(|w| w == b"\r\n\r\n") {
                    read += stream.read(&mut buf[read..]).await.expect("read request");
                }
                let reply = format!(
                    "HTTP/1.1 200 OK\r\ncontent-length: {}\r\nconnection: close\r\n\r\n{body}",
                    body.len()
                );
                stream.write_all(reply.as_bytes()).await.expect("reply");
            }
        })
    }

    #[tokio::test]
    async fn a_migrating_daemon_is_waited_for_without_spending_the_budget() {
        let _serialised = crate::test_support::fd_tests().await;
        let path = socket("migrating");
        let daemon = serve_script(&path, vec![MIGRATING, MIGRATING, MIGRATING, OK]);
        let pid = wait_for_health(&path, 1).await.expect("it came up");
        daemon.await.unwrap();
        std::fs::remove_file(&path).unwrap();
        assert_eq!(
            pid,
            Some(42),
            "a budget of one attempt outlasted three polls"
        );
    }

    #[tokio::test]
    async fn a_halted_daemon_ends_the_wait_with_its_reason() {
        let _serialised = crate::test_support::fd_tests().await;
        let path = socket("halted");
        let daemon = serve_script(&path, vec![MIGRATING, HALTED]);
        let outcome = wait_for_health(&path, 30).await;
        daemon.await.unwrap();
        std::fs::remove_file(&path).unwrap();
        assert!(
            matches!(outcome, Err(Unready::Halted(ref m)) if m == "this home is at layout 9"),
            "got {outcome:?}"
        );
    }

    #[tokio::test]
    async fn a_daemon_that_never_answers_runs_the_budget_down() {
        let _serialised = crate::test_support::fd_tests().await;
        let outcome = wait_for_health(&socket("absent"), 2).await;
        assert!(
            matches!(outcome, Err(Unready::GaveUp(2))),
            "got {outcome:?}"
        );
    }
}
