use std::path::PathBuf;

use http_body_util::{BodyExt, Full};
use hyper::Request as HyperRequest;
use hyper_util::rt::TokioIo;
use tauri::http;
use tauri::{Manager, Runtime, UriSchemeContext, UriSchemeResponder};
use tokio::net::UnixStream;

use crate::sidecar::SidecarHandle;

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
    let socket = ctx.app_handle().state::<SidecarHandle>().socket_path();

    tauri::async_runtime::spawn(async move {
        let resp = match socket {
            Some(path) => proxy(path, request)
                .await
                .unwrap_or_else(|e| error_response(502, &format!("crowbar proxy error: {e}"))),
            None => error_response(502, "crowbar daemon socket not ready"),
        };
        responder.respond(resp);
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
