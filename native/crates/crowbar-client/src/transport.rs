//! A generic blocking GET over the daemon's unix socket, for callers that
//! need more than `/v0/health`'s fixed shape.
//!
//! Added for `crowbar-sidecar` (S0.3)'s watchdog: the deep-readiness probe
//! (`GET /v0/projects`, which goes through the daemon's global view store —
//! the exact resource every observed production wedge pinned, unlike
//! `/v0/health`'s static handler) and the goroutine-dump capture
//! (`GET /debug/pprof/goroutine?debug=2`, plain text, not the envelope
//! [`crate::health`] decodes). This is the "caller that needs it" this
//! crate's own module doc comment named as the reason `/v0/health` was the
//! whole of item 0.4's transport and no more.
//!
//! §4.2 makes this crate the only one that talks to the daemon over the
//! socket, so this is the seam a supervisor reaches for instead of opening a
//! `UnixStream` and speaking HTTP/1.1 itself.

use std::fmt;
use std::path::Path;
use std::time::Duration;

/// One GET response: status code and raw body, undecoded — the two probes
/// this exists for want status-only and plain-text respectively, neither of
/// which is [`crate::health::Health`]'s JSON envelope.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RawResponse {
    pub status: u16,
    pub body: Vec<u8>,
}

impl RawResponse {
    /// Whether the response's status is 2xx.
    #[must_use]
    pub fn is_success(&self) -> bool {
        (200..300).contains(&self.status)
    }
}

/// Why a raw GET did not produce a [`RawResponse`]. Deliberately narrower than
/// [`crate::health::HealthError`]: there is no decode step here, so there is
/// no decode error.
#[derive(Debug)]
pub enum TransportError {
    /// The socket could not be dialled, or the request did not complete.
    Request(reqwest::Error),
    /// The response's status line and headers arrived but its body could not
    /// be read to completion.
    Body(reqwest::Error),
}

impl TransportError {
    /// The underlying `reqwest::Error`, whichever variant this is — the one
    /// thing a caller like the watchdog's descriptor-exhaustion check needs:
    /// walking `std::error::Error::source()` down to an `io::Error` carrying
    /// `EMFILE`/`ENFILE` does not care which stage of the request failed.
    #[must_use]
    pub fn as_reqwest_error(&self) -> &reqwest::Error {
        match self {
            Self::Request(err) | Self::Body(err) => err,
        }
    }
}

impl fmt::Display for TransportError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Request(err) => write!(f, "could not reach the daemon: {err}"),
            Self::Body(err) => write!(f, "the daemon's response body could not be read: {err}"),
        }
    }
}

impl std::error::Error for TransportError {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        Some(self.as_reqwest_error())
    }
}

/// The authority in the request URI. A unix socket has no host, but HTTP/1.1
/// requires a `Host` header and `reqwest` needs a parseable URL to build one
/// from; `reqwest`'s `unix_socket` builder skips DNS entirely, so this string
/// is never resolved. Kept in step with [`crate::health`]'s own copy —
/// duplicated rather than shared because sharing it would mean one of the two
/// modules importing the other for a single string constant.
const UNIX_AUTHORITY: &str = "http://localhost";

/// `GET {path}`, blocking, over `socket`, budgeted to `timeout`. `path` must
/// start with `/` and may carry a query string
/// (`/debug/pprof/goroutine?debug=2`).
///
/// Blocking for the same reason [`crate::health::fetch_health`] is: `gpui`
/// does not run a tokio reactor, and this is a local unix socket a caller can
/// simply run on a background thread or `cx.background_spawn`. `timeout` is a
/// required parameter rather than a default on some shared client, because
/// this function's two callers (a startup probe and a watchdog's periodic
/// deep probe) budget it completely differently and a silently-unbounded
/// default would be the wrong choice for both.
///
/// # Errors
///
/// [`TransportError`] — the daemon may legitimately be down, answer slower
/// than `timeout`, or answer with a body this call could not finish reading.
/// None of those is a fault; the status code itself is not treated as an
/// error here (unlike [`crate::health::fetch_health`]), because a caller
/// probing deep readiness needs to see a non-2xx status, not have it turned
/// into an `Err`.
pub fn get(socket: &Path, path: &str, timeout: Duration) -> Result<RawResponse, TransportError> {
    let client = reqwest::blocking::Client::builder()
        .unix_socket(socket)
        .timeout(timeout)
        .build()
        .map_err(TransportError::Request)?;

    let response = client
        .get(format!("{UNIX_AUTHORITY}{path}"))
        .send()
        .map_err(TransportError::Request)?;

    let status = response.status().as_u16();
    let body = response.bytes().map_err(TransportError::Body)?.to_vec();

    Ok(RawResponse { status, body })
}

#[cfg(test)]
mod tests {
    use std::io::{BufRead as _, BufReader, Write as _};
    use std::os::unix::net::{UnixListener, UnixStream};
    use std::path::PathBuf;
    use std::thread::JoinHandle;
    use std::time::Duration;

    use super::get;

    /// Generous enough that it is never the reason a test fails on a loaded
    /// CI box, and irrelevant to what each test actually asserts.
    const TEST_TIMEOUT: Duration = Duration::from_secs(5);

    /// A unix socket in a fresh temp directory, serving exactly one canned
    /// HTTP/1.1 response — [`crate::health`]'s own `OneShot` fixture, copied
    /// rather than shared across a `#[cfg(test)]` boundary between modules.
    struct OneShot {
        dir: PathBuf,
        socket: PathBuf,
        server: Option<JoinHandle<()>>,
    }

    impl OneShot {
        fn serving(response: String) -> Self {
            let dir = std::env::temp_dir().join(format!(
                "crowbar-transport-{}-{:?}",
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
            "{status_line}\r\nContent-Type: text/plain\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
            body.len(),
        )
    }

    #[test]
    fn a_successful_get_returns_the_status_and_raw_body() {
        let server = OneShot::serving(http("HTTP/1.1 200 OK", "goroutine dump text"));

        let response = get(
            &server.socket,
            "/debug/pprof/goroutine?debug=2",
            TEST_TIMEOUT,
        )
        .expect("served");

        assert_eq!(response.status, 200);
        assert!(response.is_success());
        assert_eq!(response.body, b"goroutine dump text");
    }

    #[test]
    fn a_non_success_status_is_returned_not_turned_into_an_error() {
        let server = OneShot::serving(http("HTTP/1.1 503 Service Unavailable", ""));

        let response =
            get(&server.socket, "/v0/projects", TEST_TIMEOUT).expect("the daemon still answered");

        assert_eq!(response.status, 503);
        assert!(!response.is_success());
    }

    #[test]
    fn nothing_listening_is_a_transport_error_not_a_panic() {
        let missing = std::env::temp_dir().join("crowbar-transport-nothing-here.sock");
        let _ = std::fs::remove_file(&missing);

        let err =
            get(&missing, "/v0/projects", TEST_TIMEOUT).expect_err("no daemon is bound there");

        assert!(err.to_string().starts_with("could not reach the daemon"));
        assert!(std::error::Error::source(&err).is_some());
    }
}
