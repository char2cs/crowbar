//! What `/v0/health` says about the daemon: serving, still preparing its home,
//! or halted. A daemon that is preparing or halted is alive on purpose; it must
//! never be mistaken for a wedge.

use std::fmt;
use std::path::PathBuf;
use std::time::Duration;

use http_body_util::BodyExt;
use serde_json::Value;
use tauri::{AppHandle, Emitter, Runtime};

use super::{http_get, supervisor};

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
    socket: &PathBuf,
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
pub async fn wait_for_health(socket: &PathBuf, attempts: u32) -> Result<Option<i32>, Unready> {
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

/// Deep readiness probe. Unlike /v0/health — which answers from a static
/// handler and stays green while the daemon is wedged — /v0/projects goes
/// through the global view store, the exact resource every observed
/// production wedge pinned. A daemon that cannot answer this is not serving
/// users, whatever its liveness endpoint says.
///
/// Dialling costs a descriptor of *this* process, so the probe reports whether it
/// could dial at all: an exhausted app cannot reach even a perfectly healthy
/// daemon, and the watchdog must not read that as the daemon's fault.
pub async fn probe_ready(socket: &PathBuf) -> supervisor::Probe {
    // A daemon preparing its home or halted refuses every route but /v0/health on
    // purpose; the deep probe would read that refusal as a wedge.
    match check_health(socket).await {
        Ok(Health::Migrating | Health::Halted { .. }) => return supervisor::Probe::Held,
        Ok(Health::Ready { .. }) => {}
        Err(e) => return dial_failure(&*e),
    }
    match http_get(socket, "/v0/projects").await {
        Ok(resp) => {
            let ok = resp.status().is_success();
            let _ = resp.into_body().collect().await;
            if ok {
                supervisor::Probe::Healthy
            } else {
                supervisor::Probe::Unserved
            }
        }
        Err(e) => dial_failure(&*e),
    }
}

fn dial_failure(err: &(dyn std::error::Error + 'static)) -> supervisor::Probe {
    if is_descriptor_exhaustion(err) {
        supervisor::Probe::LocalDescriptorExhaustion
    } else {
        supervisor::Probe::Unserved
    }
}

/// Whether a dial failed because this process is out of file descriptors. hyper
/// wraps the `connect(2)` error, so the cause chain has to be walked.
fn is_descriptor_exhaustion(err: &(dyn std::error::Error + 'static)) -> bool {
    let mut cause = Some(err);
    while let Some(e) = cause {
        if let Some(io) = e.downcast_ref::<std::io::Error>() {
            if matches!(io.raw_os_error(), Some(libc::EMFILE) | Some(libc::ENFILE)) {
                return true;
            }
        }
        cause = e.source();
    }
    false
}

#[cfg(test)]
mod tests {
    use super::super::{supervisor, PROBE_FAILURE_THRESHOLD};
    use super::{parse_health, probe_ready, wait_for_health, Health, Unready};
    use std::path::{Path, PathBuf};
    use supervisor::{probe_indicts_daemon, FailureTracker, Probe};
    use tokio::io::{AsyncReadExt, AsyncWriteExt};
    use tokio::net::UnixListener;

    // Owned by the Go side (the boot gate's and the health handler's tests assert
    // them byte for byte), so a renamed state fails here as well.
    const OK: &str = include_str!("../../../../api/internal/core/bootgate/testdata/health-ok.json");
    const MIGRATING: &str =
        include_str!("../../../../api/internal/core/bootgate/testdata/health-migrating.json");
    const HALTED: &str =
        include_str!("../../../../api/internal/core/bootgate/testdata/health-halted.json");
    const HALT_MESSAGE: &str =
        "this home is at layout 9 but this crowbar only knows layout 0: run a newer crowbar";

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

    /// A fake daemon on `path`. /v0/health answers 200 with each of `health` in
    /// turn (the last one for good after that); every other route answers `other`,
    /// as a daemon behind the boot gate refuses with 503.
    fn serve(path: &Path, health: Vec<&'static str>, other: u16) -> tokio::task::JoinHandle<()> {
        let listener = UnixListener::bind(path).expect("bind test daemon socket");
        tokio::spawn(async move {
            let mut asked = 0;
            loop {
                let (mut stream, _) = listener.accept().await.expect("accept");
                let mut buf = vec![0u8; 4096];
                let mut read = 0;
                while !buf[..read].windows(4).any(|w| w == b"\r\n\r\n") {
                    match stream.read(&mut buf[read..]).await.expect("read request") {
                        0 => break,
                        n => read += n,
                    }
                }
                let (status, body) = if buf[..read].starts_with(b"GET /v0/health ") {
                    asked += 1;
                    (200, health[(asked - 1).min(health.len() - 1)])
                } else {
                    (other, "{}")
                };
                let reply = format!(
                    "HTTP/1.1 {status} X\r\ncontent-length: {}\r\nconnection: close\r\n\r\n{body}",
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
        let daemon = serve(&path, vec![MIGRATING, MIGRATING, MIGRATING, OK], 503);
        let pid = wait_for_health(&path, 1).await.expect("it came up");
        daemon.abort();
        std::fs::remove_file(&path).unwrap();
        assert_eq!(
            pid,
            Some(4242),
            "a budget of one attempt outlasted three polls"
        );
    }

    #[tokio::test]
    async fn a_halted_daemon_ends_the_wait_with_its_reason() {
        let _serialised = crate::test_support::fd_tests().await;
        let path = socket("halted");
        let daemon = serve(&path, vec![MIGRATING, HALTED], 503);
        let outcome = wait_for_health(&path, 30).await;
        daemon.abort();
        std::fs::remove_file(&path).unwrap();
        assert!(
            matches!(outcome, Err(Unready::Halted(ref m)) if m == HALT_MESSAGE),
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

    /// Probes `path` as the watchdog does, one outcome per tick fed to its
    /// tracker, and says whether the tracker ever tripped (the road to a kill).
    async fn watch(path: &PathBuf, ticks: u32) -> (Vec<Probe>, bool) {
        let mut tracker = FailureTracker::new(PROBE_FAILURE_THRESHOLD);
        let mut outcomes = Vec::new();
        let mut tripped = false;
        for _ in 0..ticks {
            let outcome = probe_ready(path).await;
            tripped |= tracker.observe_probe(outcome);
            outcomes.push(outcome);
        }
        (outcomes, tripped)
    }

    async fn watched(tag: &str, health: &'static str, other: u16) -> (Vec<Probe>, bool) {
        let path = socket(tag);
        let daemon = serve(&path, vec![health], other);
        let seen = watch(&path, PROBE_FAILURE_THRESHOLD * 2).await;
        daemon.abort();
        std::fs::remove_file(&path).unwrap();
        seen
    }

    #[tokio::test]
    async fn a_migrating_or_halted_daemon_is_held_and_never_killed() {
        let _serialised = crate::test_support::fd_tests().await;
        for (tag, health) in [("probe-migrating", MIGRATING), ("probe-halted", HALTED)] {
            // Behind the gate every other route is 503, which a deep probe alone
            // would take for a wedge.
            let (outcomes, tripped) = watched(tag, health, 503).await;
            assert!(
                outcomes.iter().all(|o| *o == Probe::Held),
                "{tag}: {outcomes:?}"
            );
            assert!(
                !tripped,
                "{tag}: the watchdog must never trip on a held daemon"
            );
            assert!(!probe_indicts_daemon(Probe::Held));
        }
    }

    #[tokio::test]
    async fn a_serving_daemon_whose_deep_probe_fails_is_still_indicted() {
        let _serialised = crate::test_support::fd_tests().await;
        let (outcomes, tripped) = watched("probe-wedged", OK, 503).await;
        assert!(
            outcomes.iter().all(|o| *o == Probe::Unserved),
            "{outcomes:?}"
        );
        assert!(
            tripped,
            "a wedge after the daemon opened must still be caught"
        );
    }

    #[tokio::test]
    async fn a_serving_daemon_that_answers_is_healthy() {
        let _serialised = crate::test_support::fd_tests().await;
        let (outcomes, tripped) = watched("probe-healthy", OK, 200).await;
        assert!(
            outcomes.iter().all(|o| *o == Probe::Healthy),
            "{outcomes:?}"
        );
        assert!(!tripped);
    }
}
