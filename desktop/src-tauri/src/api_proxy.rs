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

/// Build an HTTP error response for the webview when proxying fails.
fn error_response(status: u16, msg: &str) -> http::Response<Vec<u8>> {
    http::Response::builder()
        .status(status)
        .header(http::header::CONTENT_TYPE, "text/plain")
        .body(msg.as_bytes().to_vec())
        .unwrap()
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
    let socket = app.state::<SidecarHandle>().socket_path();

    tauri::async_runtime::spawn(async move {
        let resp = match socket {
            // Dropping the timed-out future drops the request sender, which is what
            // tells hyper's connection task to close and hand the descriptor back.
            Some(path) => match tokio::time::timeout(PROXY_TIMEOUT, proxy(path, request)).await {
                Ok(Ok(resp)) => resp,
                Ok(Err(e)) => error_response(502, &format!("crowbar proxy error: {e}")),
                Err(_) => error_response(504, "crowbar daemon did not answer in time"),
            },
            None => error_response(502, "crowbar daemon socket not ready"),
        };
        // Respond on the main thread: WKURLSchemeTask cancellation
        // (webView:stopURLSchemeTask:) is delivered on the main thread, so
        // responding there serializes with it. Responding from a tokio worker
        // races with cancellation and a stopped task makes WebKit throw an
        // NSException that cannot unwind through the ObjC bridge -> abort().
        let _ = app.run_on_main_thread(move || responder.respond(resp));
    });
}

async fn proxy(
    socket: PathBuf,
    request: http::Request<Vec<u8>>,
) -> Result<http::Response<Vec<u8>>, Box<dyn std::error::Error + Send + Sync>> {
    // The custom-scheme URI is `crowbar://localhost/v0/...`. hyper only needs
    // the path-and-query portion for the request line over a unix socket.
    let path_and_query = request
        .uri()
        .path_and_query()
        .map(|pq| pq.as_str().to_string())
        .unwrap_or_else(|| request.uri().path().to_string());

    let stream = UnixStream::connect(&socket).await?;
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
