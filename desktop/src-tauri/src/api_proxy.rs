use std::future::Future;
use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;

use http_body_util::{BodyExt, Full};
use hyper::Request as HyperRequest;
use hyper_util::rt::TokioIo;
use tauri::http;
use tauri::{Manager, Runtime, UriSchemeContext, UriSchemeResponder};
use tokio::io::{AsyncRead, AsyncWrite};
use tokio::net::UnixStream;

use crate::host_tokens::TokenStore;
use crate::hosts::HostRegistry;
use crate::sidecar::SidecarHandle;

pub(crate) mod pairing;
pub(crate) mod probe;
pub(crate) mod remote;
#[cfg(test)]
pub(crate) mod test_support;

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
/// the schedule of `connect_retry_delay` between them (~4.5s in total). This is
/// the desktop's cold-start analogue of the retry in web/src/lib/api.ts, which only
/// fires when `fetch` REJECTS: over the vite transport a connect refused does
/// exactly that, but through this proxy it used to become a real HTTP 502, which
/// the frontend rightly treats as a terminal daemon answer. The window is real —
/// `socket_path` is published before the daemon has bound, and a watchdog respawn
/// leaves it published while nothing listens — so the transport retries here,
/// where the transport is.
///
/// The first second polls every 25ms so a daemon that binds within a few
/// hundred ms is reached promptly; the exponential tail covers a slow start.
const CONNECT_ATTEMPTS: u32 = CONNECT_FAST_STEPS + 7;
const CONNECT_FAST_STEPS: u32 = 40;
const CONNECT_FAST_DELAY: Duration = Duration::from_millis(25);
const CONNECT_RETRY_BASE: Duration = Duration::from_millis(100);
const CONNECT_RETRY_CAP: Duration = Duration::from_secs(1);

/// Marks a response that is the PROXY's, not the daemon's; the value says why.
pub(crate) const PROXY_HEADER: &str = "x-crowbar-proxy";
/// The local daemon socket: nothing was listening.
pub(crate) const DAEMON_UNAVAILABLE: &str = "daemon-unavailable";
/// A remote host did not answer: refused, timed out, or dropped the handshake.
pub(crate) const HOST_UNREACHABLE: &str = "host-unreachable";
/// A remote host answered, but not with the identity pinned at pairing.
pub(crate) const HOST_IDENTITY_CHANGED: &str = "host-identity-changed";

/// Tags an answer the proxy wrote itself rather than relayed from a daemon.
#[derive(Clone, Copy)]
struct ProxyAnswer;

/// Build an HTTP error response for the webview when proxying fails.
fn error_response(status: u16, msg: &str) -> http::Response<Vec<u8>> {
    let mut resp = http::Response::builder()
        .status(status)
        .header(http::header::CONTENT_TYPE, "text/plain")
        .body(msg.as_bytes().to_vec())
        .unwrap();
    resp.extensions_mut().insert(ProxyAnswer);
    resp
}

fn proxy_error(status: u16, marker: &'static str, msg: &str) -> http::Response<Vec<u8>> {
    let mut resp = error_response(status, msg);
    resp.headers_mut()
        .insert(PROXY_HEADER, http::HeaderValue::from_static(marker));
    resp
}

/// The page reaches `crowbar://` cross-origin, so WebKit hides any answer
/// without CORS headers — status and marker too — and `fetch` rejects opaquely.
/// A daemon grants its own answers; the proxy's are granted here.
fn readable_by(
    origin: Option<http::HeaderValue>,
    mut resp: http::Response<Vec<u8>>,
) -> http::Response<Vec<u8>> {
    let Some(origin) = origin else {
        return resp;
    };
    if resp.extensions().get::<ProxyAnswer>().is_none() {
        return resp;
    }
    let headers = resp.headers_mut();
    headers.insert(http::header::ACCESS_CONTROL_ALLOW_ORIGIN, origin);
    headers.insert(
        http::header::ACCESS_CONTROL_EXPOSE_HEADERS,
        http::HeaderValue::from_static(PROXY_HEADER),
    );
    headers.append(http::header::VARY, http::HeaderValue::from_static("Origin"));
    resp
}

fn daemon_unavailable(msg: &str) -> http::Response<Vec<u8>> {
    proxy_error(503, DAEMON_UNAVAILABLE, msg)
}

fn connect_retry_delay(attempt: u32) -> Duration {
    if attempt <= CONNECT_FAST_STEPS {
        return CONNECT_FAST_DELAY;
    }
    let tail = attempt - CONNECT_FAST_STEPS - 1;
    (CONNECT_RETRY_BASE * 2u32.saturating_pow(tail.min(8))).min(CONNECT_RETRY_CAP)
}

/// Mirrors api.ts `isIdempotentRead`: mutations are never replayed.
fn is_idempotent_read(method: &http::Method) -> bool {
    matches!(*method, http::Method::GET | http::Method::HEAD)
}

/// `localhost` (or no host at all) is the local daemon; any other name is the id
/// of a paired remote host. Both transports route by this.
pub(crate) fn is_remote_host(host: &str) -> bool {
    !host.is_empty() && host != "localhost"
}

/// `crowbar://localhost/...` is the local daemon; any other authority is the id
/// of a paired remote host.
fn remote_host_id(uri: &http::Uri) -> Option<&str> {
    uri.host().filter(|host| is_remote_host(host))
}

/// Async URI-scheme handler for the `crowbar://` scheme. Incoming requests look
/// like `crowbar://localhost/v0/...`; we forward the method, path+query,
/// headers and body to the daemon over its unix socket and relay the response.
/// `crowbar://<hostId>/v0/...` goes to that paired remote host instead.
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
    let remote_host = remote_host_id(request.uri()).map(str::to_string);
    let origin = request.headers().get(http::header::ORIGIN).cloned();

    tauri::async_runtime::spawn(async move {
        // Dropping the timed-out future drops the request sender, which is what
        // tells hyper's connection task to close and hand the descriptor back.
        let forwarded = async {
            match remote_host {
                None => {
                    forward(
                        || state_app.state::<SidecarHandle>().socket_path(),
                        request,
                        |attempt| tokio::time::sleep(connect_retry_delay(attempt)),
                    )
                    .await
                }
                Some(host_id) => {
                    remote::forward_remote(
                        &state_app.state::<HostRegistry>(),
                        &state_app.state::<Arc<dyn TokenStore>>(),
                        &host_id,
                        request,
                    )
                    .await
                }
            }
        };
        let resp = match tokio::time::timeout(PROXY_TIMEOUT, forwarded).await {
            Ok(resp) => resp,
            Err(_) => error_response(504, "crowbar daemon did not answer in time"),
        };
        let resp = readable_by(origin, resp);
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

async fn proxy<S>(
    stream: S,
    request: http::Request<Vec<u8>>,
) -> Result<http::Response<Vec<u8>>, Box<dyn std::error::Error + Send + Sync>>
where
    S: AsyncRead + AsyncWrite + Unpin + Send + 'static,
{
    // The custom-scheme URI is `crowbar://<authority>/v0/...`. hyper only needs
    // the path-and-query portion for the request line.
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
        // The marker is the proxy's word alone; a daemon's copy would pass for it.
        let relayed = resp_parts
            .headers
            .iter()
            .filter(|(name, _)| *name != PROXY_HEADER);
        for (name, value) in relayed {
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

    use super::test_support::{serve_once_unix, tempdir};
    use super::{
        connect_retry_delay, forward, readable_by, remote_host_id, CONNECT_ATTEMPTS,
        DAEMON_UNAVAILABLE, PROXY_HEADER,
    };

    const PAGE: &str = "tauri://localhost";

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

    #[test]
    fn connect_retry_delay_polls_fast_then_backs_off_within_the_window() {
        assert_eq!(connect_retry_delay(1), Duration::from_millis(25));
        assert_eq!(connect_retry_delay(40), Duration::from_millis(25));
        assert_eq!(connect_retry_delay(41), Duration::from_millis(100));
        assert_eq!(connect_retry_delay(44), Duration::from_millis(800));
        assert_eq!(connect_retry_delay(45), Duration::from_secs(1));
        let total: Duration = (1..CONNECT_ATTEMPTS).map(connect_retry_delay).sum();
        assert_eq!(total, Duration::from_millis(4500));
    }

    // Regression: a GET issued while the daemon is still binding (or mid-respawn)
    // must reach it once it listens, instead of answering the webview a 502 it
    // treats as terminal.
    #[tokio::test]
    async fn get_retries_the_connect_until_the_daemon_listens() {
        let _serialised = crate::test_support::fd_tests().await;
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
                        serve_once_unix(bind_path.clone());
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
        let _serialised = crate::test_support::fd_tests().await;
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
        let _serialised = crate::test_support::fd_tests().await;
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
                    serve_once_unix(bind_path.clone());
                    published.lock().unwrap().replace(bind_path.clone());
                    async {}
                },
            )
            .await
        };
        assert_eq!(resp.status(), 200);
    }

    fn cors(resp: &http::Response<Vec<u8>>, name: http::header::HeaderName) -> Option<&str> {
        resp.headers().get(name).map(|v| v.to_str().unwrap())
    }

    // Regression: without these, WebKit rejected the fetch with an opaque
    // "Load failed" and the marker never reached the page.
    #[tokio::test]
    async fn a_proxy_answer_is_readable_by_the_page_that_asked() {
        let resp = forward(|| None, request(http::Method::POST), |_| async {}).await;

        let resp = readable_by(Some(http::HeaderValue::from_static(PAGE)), resp);

        assert_eq!(resp.status(), 503);
        let allow_origin = http::header::ACCESS_CONTROL_ALLOW_ORIGIN;
        assert_eq!(cors(&resp, allow_origin), Some(PAGE));
        let exposed = cors(&resp, http::header::ACCESS_CONTROL_EXPOSE_HEADERS);
        assert_eq!(exposed, Some(PROXY_HEADER));
        assert_eq!(cors(&resp, http::header::VARY), Some("Origin"));
        assert_eq!(
            cors(&resp, PROXY_HEADER.parse().unwrap()),
            Some(DAEMON_UNAVAILABLE)
        );
    }

    #[tokio::test]
    async fn a_daemon_answer_keeps_its_own_cors_decision() {
        let _serialised = crate::test_support::fd_tests().await;
        let dir = tempdir::Dir::new("cors");
        let path = socket_in(&dir);
        serve_once_unix(path.clone());

        let resp = forward(
            || Some(path.clone()),
            request(http::Method::GET),
            |_| async {},
        )
        .await;
        let resp = readable_by(Some(http::HeaderValue::from_static(PAGE)), resp);

        assert_eq!(resp.status(), 200);
        let allow_origin = http::header::ACCESS_CONTROL_ALLOW_ORIGIN;
        assert_eq!(
            cors(&resp, allow_origin),
            None,
            "the daemon did not grant it"
        );
    }

    #[test]
    fn only_a_non_localhost_authority_routes_to_a_remote_host() {
        let host = |uri: &'static str| remote_host_id(&uri.parse().unwrap()).map(str::to_string);
        assert_eq!(host("crowbar://localhost/v0/projects"), None);
        assert_eq!(host("crowbar://localhost:80/v0/projects"), None);
        assert_eq!(host("crowbar://host-1/v0/projects"), Some("host-1".into()));
    }
}
