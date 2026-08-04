//! `GET /v0/health` over the daemon's unix socket.
//!
//! Deliberately the whole of the transport for now (item 0.4): one request, no
//! connection pool worth the name, no WebSocket, no reconnect. §9.1's full
//! client — HTTP, WS, reconnect and backoff — is a later item, and building it
//! ahead of a caller would be building it blind.

use std::fmt;
use std::path::Path;

use serde::Deserialize;

/// The daemon's `/v0/health` payload.
///
/// Mirrors `api/internal/api/v0/endpoints/health/handlers/health.go`, which
/// answers `{"status","version","pid"}` inside the standard query envelope.
/// It lives here rather than in `crowbar-proto` because that crate is
/// *generated* from the Go handlers (§9.2) and hand-writing into it would be
/// overwritten by the generator. It moves there when the generator runs.
#[derive(Debug, Clone, PartialEq, Eq, Deserialize)]
pub struct Health {
    /// The daemon's self-reported pid. Load-bearing on the desktop side: the
    /// supervisor signals the daemon by this number rather than by the child
    /// handle, which deadlocks.
    pub pid: i64,
    /// `"ok"` from a live daemon. Carried as a string rather than a bool
    /// because the daemon is free to grow degraded states.
    pub status: String,
    /// The daemon build version.
    pub version: String,
}

/// The daemon's uniform response envelope: `{"success":…,"data":…}`.
#[derive(Debug, Deserialize)]
struct Envelope<T> {
    data: T,
}

/// Why a health probe did not produce a [`Health`].
///
/// Every variant is a state a *correct* client reaches — the daemon is
/// routinely down while the app is starting, and a version skew is a
/// deserialise failure. None of them is a panic.
#[derive(Debug)]
pub enum HealthError {
    /// The socket could not be dialled, or the request did not complete. On a
    /// unix socket this is nearly always "no daemon is listening there".
    Transport(reqwest::Error),
    /// The daemon answered, but not with success.
    Status(u16),
    /// The daemon answered with a body this client could not read as a
    /// [`Health`] — a schema skew between the two halves.
    Decode(serde_json::Error),
}

impl fmt::Display for HealthError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Transport(err) => write!(f, "could not reach the daemon: {err}"),
            Self::Status(code) => write!(f, "the daemon answered /v0/health with HTTP {code}"),
            Self::Decode(err) => write!(f, "the daemon's /v0/health body did not parse: {err}"),
        }
    }
}

impl std::error::Error for HealthError {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        match self {
            Self::Transport(err) => Some(err),
            Self::Decode(err) => Some(err),
            Self::Status(_) => None,
        }
    }
}

/// The authority in the request URI.
///
/// A unix socket has no host, but HTTP/1.1 requires a `Host` header and
/// `reqwest` needs a parseable URL to build one from. `reqwest`'s
/// `unix_socket` builder skips DNS entirely, so this string is never resolved.
const UNIX_AUTHORITY: &str = "http://localhost";

/// `GET /v0/health`, blocking, over `socket`.
///
/// Blocking on purpose. `gpui` does not run a tokio reactor, and threading an
/// async runtime through the app to reach a local unix socket would buy
/// nothing: call this from `cx.background_spawn` and the executor gpui already
/// owns does the waiting.
///
/// # Errors
///
/// [`HealthError`] — the daemon may legitimately be down, answer non-2xx, or
/// answer with a body this build cannot read. All three are reportable states,
/// not faults.
pub fn fetch_health(socket: &Path) -> Result<Health, HealthError> {
    fetch_health_inner(socket, None)
}

/// [`fetch_health`], budgeted to `timeout`. Added for `crowbar-sidecar`
/// (S0.3): the original's startup probe and adopt-if-healthy check both bound
/// every attempt (a booting daemon answers immediately or not at all, so this
/// only has to outlast a loaded machine, never a slow request), which the
/// unbounded [`fetch_health`] cannot express. Kept as a sibling rather than
/// changed into it, so `fetch_health`'s existing behaviour for its existing
/// callers (`crowbar-app`'s own startup probe, item 0.4) does not shift under
/// them as a side effect of this addition.
///
/// # Errors
///
/// Same as [`fetch_health`], plus [`HealthError::Transport`] when the request
/// does not complete within `timeout`.
pub fn fetch_health_with_timeout(
    socket: &Path,
    timeout: std::time::Duration,
) -> Result<Health, HealthError> {
    fetch_health_inner(socket, Some(timeout))
}

fn fetch_health_inner(
    socket: &Path,
    timeout: Option<std::time::Duration>,
) -> Result<Health, HealthError> {
    let mut builder = reqwest::blocking::Client::builder().unix_socket(socket);
    if let Some(timeout) = timeout {
        builder = builder.timeout(timeout);
    }
    let client = builder.build().map_err(HealthError::Transport)?;

    let response = client
        .get(format!("{UNIX_AUTHORITY}/v0/health"))
        .send()
        .map_err(HealthError::Transport)?;

    let status = response.status();
    let body = response.text().map_err(HealthError::Transport)?;

    if !status.is_success() {
        return Err(HealthError::Status(status.as_u16()));
    }

    serde_json::from_str::<Envelope<Health>>(&body)
        .map(|envelope| envelope.data)
        .map_err(HealthError::Decode)
}

#[cfg(test)]
mod tests {
    use std::io::{BufRead as _, BufReader, Write as _};
    use std::os::unix::net::{UnixListener, UnixStream};
    use std::path::PathBuf;
    use std::thread::JoinHandle;

    use super::*;

    /// A unix socket in a fresh temp directory, serving exactly one canned
    /// HTTP/1.1 response.
    ///
    /// The listener is bound *before* the serving thread starts, so a client
    /// that dials immediately cannot lose a race — there is nothing here to
    /// sleep on or poll for.
    struct OneShot {
        dir: PathBuf,
        socket: PathBuf,
        server: Option<JoinHandle<()>>,
    }

    impl OneShot {
        fn serving(response: String) -> Self {
            // A short path: macOS caps sun_path at 104 bytes, and this
            // repository's own worktree paths are long enough to blow it.
            let dir = std::env::temp_dir().join(format!(
                "crowbar-health-{}-{:?}",
                std::process::id(),
                std::thread::current().id(),
            ));
            std::fs::create_dir_all(&dir).expect("temp dir");
            let socket = dir.join("d.sock");
            let listener = UnixListener::bind(&socket).expect("bind");

            let server = std::thread::spawn(move || {
                let Ok((stream, _)) = listener.accept() else {
                    return;
                };
                Self::answer(&stream, &response);
            });

            Self {
                dir,
                socket,
                server: Some(server),
            }
        }

        /// Reads the request head (up to the blank line) and writes `response`.
        /// Draining the head matters: hyper will not surface the response if
        /// the peer never read the request.
        fn answer(stream: &UnixStream, response: &str) {
            let mut reader = BufReader::new(stream);
            let mut line = String::new();
            while reader.read_line(&mut line).unwrap_or(0) > 0 {
                if line == "\r\n" || line == "\n" {
                    break;
                }
                line.clear();
            }
            let mut out = stream;
            let _ = out.write_all(response.as_bytes());
            let _ = out.flush();
        }
    }

    impl Drop for OneShot {
        fn drop(&mut self) {
            if let Some(server) = self.server.take() {
                let _ = server.join();
            }
            let _ = std::fs::remove_dir_all(&self.dir);
        }
    }

    fn http(status_line: &str, body: &str) -> String {
        format!(
            "{status_line}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
            body.len(),
        )
    }

    #[test]
    fn a_live_daemon_yields_pid_status_and_version() {
        let body = r#"{"success":true,"data":{"status":"ok","version":"0.1.0-dev","pid":62909}}"#;
        let server = OneShot::serving(http("HTTP/1.1 200 OK", body));

        let health = fetch_health(&server.socket).expect("the canned daemon answered");

        assert_eq!(
            health,
            Health {
                pid: 62909,
                status: "ok".to_owned(),
                version: "0.1.0-dev".to_owned(),
            }
        );
        assert!(!format!("{health:?}").is_empty());
        assert_eq!(health.clone(), health);
    }

    #[test]
    fn fetch_health_with_timeout_reaches_the_same_daemon_as_fetch_health() {
        let body = r#"{"success":true,"data":{"status":"ok","version":"0.1.0-dev","pid":1}}"#;
        let server = OneShot::serving(http("HTTP/1.1 200 OK", body));

        let health = fetch_health_with_timeout(&server.socket, std::time::Duration::from_secs(5))
            .expect("the canned daemon answered");

        assert_eq!(health.pid, 1);
    }

    #[test]
    fn nothing_listening_is_a_transport_error_not_a_panic() {
        let missing = std::env::temp_dir().join("crowbar-health-nothing-here.sock");
        let _ = std::fs::remove_file(&missing);

        let err = fetch_health(&missing).expect_err("no daemon is bound there");

        assert!(matches!(err, HealthError::Transport(_)));
        assert!(err.to_string().starts_with("could not reach the daemon"));
        assert!(std::error::Error::source(&err).is_some());
    }

    #[test]
    fn a_non_success_status_is_reported_with_its_code() {
        let server = OneShot::serving(http(
            "HTTP/1.1 503 Service Unavailable",
            r#"{"success":false}"#,
        ));

        let err = fetch_health(&server.socket).expect_err("503 is not health");

        assert!(matches!(err, HealthError::Status(503)));
        assert_eq!(
            err.to_string(),
            "the daemon answered /v0/health with HTTP 503",
        );
        assert!(std::error::Error::source(&err).is_none());
    }

    #[test]
    fn a_schema_skew_is_reported_as_a_decode_failure() {
        // `pid` as a string is exactly the shape a drifted daemon would send.
        let body = r#"{"success":true,"data":{"status":"ok","version":"9","pid":"62909"}}"#;
        let server = OneShot::serving(http("HTTP/1.1 200 OK", body));

        let err = fetch_health(&server.socket).expect_err("pid is not a string");

        assert!(matches!(err, HealthError::Decode(_)));
        assert!(err.to_string().starts_with("the daemon's /v0/health body"));
        assert!(std::error::Error::source(&err).is_some());
        assert!(!format!("{err:?}").is_empty());
    }
}
