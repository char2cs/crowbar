use std::future::Future;
use std::path::PathBuf;
use std::time::Duration;

use http_body_util::{BodyExt, Full};
use hyper::Request as HyperRequest;
use hyper_util::rt::TokioIo;
use tauri::http;
use tauri::{Manager, Runtime, UriSchemeContext, UriSchemeResponder};
use tokio::net::UnixStream;

use crate::sidecar::SidecarHandle;

/// Upper bound on one proxied request.
///
/// Each request costs this process a unix socket, and a daemon that accepts a
/// connection but never answers — the wedge the health watchdog exists for — would
/// otherwise pin that descriptor, and the two tasks driving it, for the life of the
/// app. Nothing else would ever release them: there is no cap on requests in
/// flight, and the frontend's `fetch` has no AbortController, so it never gives up
/// either.
///
/// Comfortably above anything the daemon can legitimately take: it bounds its own
/// slowest work — a network git transfer — at 3 minutes (`netTransferTimeout`), and
/// it never clones, so no honest request outlives that.
const PROXY_TIMEOUT: Duration = Duration::from_secs(300);

/// Connect attempts for an idempotent read that cannot reach the daemon, with
/// the backoff of `connect_retry_delay` between them (~4.5s in total). This is
/// the desktop's copy of the cold-start retry in web/src/lib/api.ts, which only
/// fires when `fetch` REJECTS: over the vite transport a connect refused does
/// exactly that, but through this proxy it used to become a real HTTP 502, which
/// the frontend rightly treats as a terminal daemon answer. The window is real —
/// `socket_path` is published before the daemon has bound, and a watchdog respawn
/// leaves it published while nothing listens — so the transport retries here,
/// where the transport is.
const CONNECT_ATTEMPTS: u32 = 8;
const CONNECT_RETRY_BASE: Duration = Duration::from_millis(100);
const CONNECT_RETRY_CAP: Duration = Duration::from_secs(1);

/// Marks a response that is the PROXY's, not the daemon's: nothing was listening.
pub(crate) const PROXY_HEADER: &str = "x-crowbar-proxy";
pub(crate) const DAEMON_UNAVAILABLE: &str = "daemon-unavailable";

/// Build an HTTP error response for the webview when proxying fails.
fn error_response(status: u16, msg: &str) -> http::Response<Vec<u8>> {
    http::Response::builder()
        .status(status)
        .header(http::header::CONTENT_TYPE, "text/plain")
        .body(msg.as_bytes().to_vec())
        .unwrap()
}

fn daemon_unavailable(msg: &str) -> http::Response<Vec<u8>> {
    http::Response::builder()
        .status(503)
        .header(http::header::CONTENT_TYPE, "text/plain")
        .header(PROXY_HEADER, DAEMON_UNAVAILABLE)
        .body(msg.as_bytes().to_vec())
        .unwrap()
}

fn connect_retry_delay(attempt: u32) -> Duration {
    (CONNECT_RETRY_BASE * 2u32.pow(attempt.saturating_sub(1))).min(CONNECT_RETRY_CAP)
}

/// Mirrors api.ts `isIdempotentRead`: mutations are never replayed.
fn is_idempotent_read(method: &http::Method) -> bool {
    matches!(*method, http::Method::GET | http::Method::HEAD)
}

/// Async URI-scheme handler for the `crowbar://` scheme. Incoming requests look
/// like `crowbar://localhost/v0/...`; we forward the method, path+query,
/// headers and body to the daemon over its unix socket and relay the response.
///
/// Registered on the Tauri builder with
/// `.register_asynchronous_uri_scheme_protocol("crowbar", handle_request)`.
pub fn handle_request<R: Runtime>(
    ctx: UriSchemeContext<'_, R>,
    request: http::Request<Vec<u8>>,
    responder: UriSchemeResponder,
) {
    let app = ctx.app_handle().clone();
    let state_app = app.clone();

    tauri::async_runtime::spawn(async move {
        // Dropping the timed-out future drops the request sender, which is what
        // tells hyper's connection task to close and hand the descriptor back.
        let forwarded = forward(
            || state_app.state::<SidecarHandle>().socket_path(),
            request,
            |attempt| tokio::time::sleep(connect_retry_delay(attempt)),
        );
        let resp = match tokio::time::timeout(PROXY_TIMEOUT, forwarded).await {
            Ok(resp) => resp,
            Err(_) => error_response(504, "crowbar daemon did not answer in time"),
        };
        // Respond on the main thread: WKURLSchemeTask cancellation
        // (webView:stopURLSchemeTask:) is delivered on the main thread, so
        // responding there serializes with it. Responding from a tokio worker
        // races with cancellation and a stopped task makes WebKit throw an
        // NSException that cannot unwind through the ObjC bridge -> abort().
        let _ = app.run_on_main_thread(move || responder.respond(resp));
    });
}

/// Connects to the daemon — re-reading the socket path each time, since it is
/// published asynchronously — and relays the request. Only the CONNECT can be
/// retried: a failure there proves the request never reached the daemon.
async fn forward<S, W, F>(
    socket: S,
    request: http::Request<Vec<u8>>,
    mut wait: W,
) -> http::Response<Vec<u8>>
where
    S: Fn() -> Option<PathBuf>,
    W: FnMut(u32) -> F,
    F: Future<Output = ()>,
{
    let retryable = is_idempotent_read(request.method());
    let mut attempt = 1;
    let stream = loop {
        let failure = match socket() {
            Some(path) => match UnixStream::connect(&path).await {
                Ok(stream) => break stream,
                Err(e) => format!("crowbar daemon socket {}: {e}", path.display()),
            },
            None => "crowbar daemon socket not ready".to_string(),
        };
        if !retryable || attempt >= CONNECT_ATTEMPTS {
            return daemon_unavailable(&failure);
        }
        wait(attempt).await;
        attempt += 1;
    };
    match proxy(stream, request).await {
        Ok(resp) => resp,
        Err(e) => error_response(502, &format!("crowbar proxy error: {e}")),
    }
}

async fn proxy(
    stream: UnixStream,
    request: http::Request<Vec<u8>>,
) -> Result<http::Response<Vec<u8>>, Box<dyn std::error::Error + Send + Sync>> {
    // The custom-scheme URI is `crowbar://localhost/v0/...`. hyper only needs
    // the path-and-query portion for the request line over a unix socket.
    let path_and_query = request
        .uri()
        .path_and_query()
        .map(|pq| pq.as_str().to_string())
        .unwrap_or_else(|| request.uri().path().to_string());

    let io = TokioIo::new(stream);
    let (mut sender, conn) = hyper::client::conn::http1::handshake(io).await?;
    tokio::spawn(async move {
        let _ = conn.await;
    });

    let (parts, body) = request.into_parts();

    let mut builder = HyperRequest::builder()
        .method(parts.method)
        .uri(path_and_query);

    // Copy through the incoming headers; ensure a Host header exists for HTTP/1.1.
    if let Some(headers) = builder.headers_mut() {
        for (name, value) in parts.headers.iter() {
            headers.insert(name, value.clone());
        }
        if !headers.contains_key(http::header::HOST) {
            headers.insert(
                http::header::HOST,
                http::HeaderValue::from_static("localhost"),
            );
        }
    }

    let upstream_req = builder.body(Full::<bytes::Bytes>::new(body.into()))?;

    let upstream_resp = sender.send_request(upstream_req).await?;
    let (resp_parts, resp_body) = upstream_resp.into_parts();
    let collected = resp_body.collect().await?.to_bytes().to_vec();

    let mut out = http::Response::builder().status(resp_parts.status);
    if let Some(headers) = out.headers_mut() {
        for (name, value) in resp_parts.headers.iter() {
            headers.insert(name, value.clone());
        }
    }
    Ok(out.body(collected)?)
}

#[cfg(test)]
mod tests {
    use std::path::PathBuf;
    use std::sync::atomic::{AtomicU32, Ordering};
    use std::sync::Arc;
    use std::time::Duration;

    use tauri::http;
    use tokio::io::{AsyncReadExt, AsyncWriteExt};
    use tokio::net::UnixListener;

    use super::{connect_retry_delay, forward, CONNECT_ATTEMPTS, DAEMON_UNAVAILABLE, PROXY_HEADER};

    fn request(method: http::Method) -> http::Request<Vec<u8>> {
        http::Request::builder()
            .method(method)
            .uri("crowbar://localhost/v0/projects")
            .body(Vec::new())
            .unwrap()
    }

    fn socket_in(dir: &tempdir::Dir) -> PathBuf {
        dir.path.join("s.sock")
    }

    /// Binds `path` and answers exactly one HTTP/1.1 request with `200 ok`.
    fn serve_once(path: PathBuf) {
        let listener = UnixListener::bind(&path).expect("bind test daemon socket");
        tokio::spawn(async move {
            let (mut stream, _) = listener.accept().await.expect("accept");
            let mut buf = vec![0u8; 4096];
            let mut read = 0;
            loop {
                let n = stream.read(&mut buf[read..]).await.expect("read request");
                read += n;
                if n == 0 || buf[..read].windows(4).any(|w| w == b"\r\n\r\n") {
                    break;
                }
            }
            stream
                .write_all(b"HTTP/1.1 200 OK\r\ncontent-length: 2\r\nconnection: close\r\n\r\nok")
                .await
                .expect("write response");
        });
    }

    #[test]
    fn connect_retry_delay_matches_the_web_schedule() {
        assert_eq!(connect_retry_delay(1), Duration::from_millis(100));
        assert_eq!(connect_retry_delay(2), Duration::from_millis(200));
        assert_eq!(connect_retry_delay(4), Duration::from_millis(800));
        assert_eq!(connect_retry_delay(5), Duration::from_secs(1));
        assert_eq!(connect_retry_delay(8), Duration::from_secs(1));
    }

    // Regression: a GET issued while the daemon is still binding (or mid-respawn)
    // must reach it once it listens, instead of answering the webview a 502 it
    // treats as terminal.
    #[tokio::test]
    async fn get_retries_the_connect_until_the_daemon_listens() {
        let dir = tempdir::Dir::new("retry");
        let path = socket_in(&dir);
        let waits = Arc::new(AtomicU32::new(0));
        let resp = {
            let bind_path = path.clone();
            let waits = waits.clone();
            forward(
                || Some(path.clone()),
                request(http::Method::GET),
                move |_attempt| {
                    // The first retry IS the signal the daemon binds on.
                    if waits.fetch_add(1, Ordering::SeqCst) == 0 {
                        serve_once(bind_path.clone());
                    }
                    async {}
                },
            )
            .await
        };
        assert_eq!(resp.status(), 200);
        assert_eq!(resp.body(), b"ok");
        assert_eq!(waits.load(Ordering::SeqCst), 1);
    }

    #[tokio::test]
    async fn get_gives_up_after_the_budget_with_a_marked_503() {
        let dir = tempdir::Dir::new("budget");
        let path = socket_in(&dir);
        let waits = Arc::new(AtomicU32::new(0));
        let resp = {
            let waits = waits.clone();
            forward(
                || Some(path.clone()),
                request(http::Method::GET),
                move |_attempt| {
                    waits.fetch_add(1, Ordering::SeqCst);
                    async {}
                },
            )
            .await
        };
        assert_eq!(resp.status(), 503);
        assert_eq!(
            resp.headers()
                .get(PROXY_HEADER)
                .map(|v| v.to_str().unwrap()),
            Some(DAEMON_UNAVAILABLE)
        );
        assert_eq!(waits.load(Ordering::SeqCst), CONNECT_ATTEMPTS - 1);
    }

    #[tokio::test]
    async fn mutation_is_never_replayed_and_is_marked_unavailable() {
        let resp = forward(
            || None,
            request(http::Method::POST),
            |_attempt| async { panic!("a POST must not be retried") },
        )
        .await;
        assert_eq!(resp.status(), 503);
        assert_eq!(
            resp.headers()
                .get(PROXY_HEADER)
                .map(|v| v.to_str().unwrap()),
            Some(DAEMON_UNAVAILABLE)
        );
    }

    #[tokio::test]
    async fn socket_published_after_the_first_attempt_is_picked_up() {
        let dir = tempdir::Dir::new("late");
        let path = socket_in(&dir);
        let published = Arc::new(std::sync::Mutex::new(None::<PathBuf>));
        let resp = {
            let published_reader = published.clone();
            let bind_path = path.clone();
            forward(
                move || published_reader.lock().unwrap().clone(),
                request(http::Method::GET),
                move |_attempt| {
                    serve_once(bind_path.clone());
                    published.lock().unwrap().replace(bind_path.clone());
                    async {}
                },
            )
            .await
        };
        assert_eq!(resp.status(), 200);
    }

    mod tempdir {
        use std::path::PathBuf;

        pub struct Dir {
            pub path: PathBuf,
        }

        impl Dir {
            // Short on purpose: a unix socket path must fit SUN_LEN (104 bytes on
            // macOS) and the system temp dir already spends half of that.
            pub fn new(tag: &str) -> Self {
                let path = std::env::temp_dir().join(format!(
                    "cb-{tag}-{}-{}",
                    std::process::id(),
                    std::time::SystemTime::now()
                        .duration_since(std::time::UNIX_EPOCH)
                        .unwrap()
                        .subsec_nanos()
                ));
                std::fs::create_dir_all(&path).unwrap();
                Self { path }
            }
        }

        impl Drop for Dir {
            fn drop(&mut self) {
                let _ = std::fs::remove_dir_all(&self.path);
            }
        }
    }
}
