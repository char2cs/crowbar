//! Generic WebSocket-over-unix-socket bridge for the desktop app.
//!
//! The webview cannot open a `WebSocket` to the daemon: its only reachable
//! endpoint is the `crowbar://` unix-socket proxy (see `api_proxy`), and the
//! browser `WebSocket` constructor rejects every scheme but `ws`/`wss`. So Rust
//! is the WebSocket client here — it dials the daemon's existing unix socket
//! (where a WS upgrade is perfectly legal) and bridges an arbitrary `/v0/...`
//! WebSocket route both ways to the webview.
//!
//!   * daemon → webview: each text frame is pushed verbatim down the Tauri
//!     `Channel` the frontend supplies at open time (it arrives as a string), each
//!     binary frame as raw bytes (it arrives as an `ArrayBuffer`) — the terminal's
//!     PTY output is binary, everything else is JSON text.
//!   * webview → daemon: `ws_send` enqueues the supplied text frame for the
//!     connection's writer task.
//!
//! Connections are keyed by a client-supplied id (`crypto.randomUUID` on the JS
//! side) so multiple scoped streams — and several windows attached to one PTY —
//! coexist without ever colliding.
//!
//! This is the ONE bridge: every WebSocket route, the terminal's PTY stream
//! included, goes through it. How a connection lives and dies is in `connection`.

use std::path::Path;
use std::sync::Arc;
use std::time::Duration;

use tauri::ipc::{Channel, InvokeResponseBody};
use tauri::{Manager, State};
use tokio::io::{AsyncRead, AsyncWrite};
use tokio::net::UnixStream;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;
use tokio_tungstenite::tungstenite::handshake::client::Request;
use tokio_tungstenite::tungstenite::Message;
use tokio_tungstenite::{client_async, WebSocketStream};

use crate::host_tokens::TokenStore;
use crate::hosts::HostRegistry;

mod connection;
mod remote;
#[cfg(test)]
mod test_support;

use connection::attach;
pub use connection::WsBridgeManager;

/// Sentinel pushed down the message Channel when the daemon closes the stream
/// (close frame, error, or EOF). The JS shim recognises it and fires `onclose`
/// so `wsManager` reconnects and the §6 cache re-seeds. A NUL prefix makes it
/// impossible to collide with a real JSON DTO frame (which always starts `{`).
pub const WS_CLOSE_SENTINEL: &str = "\u{0}crowbar-ws-close";

/// One daemon → webview frame.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Frame {
    /// A text frame (a JSON DTO), or the close sentinel.
    Text(String),
    /// A binary frame (terminal PTY output), forwarded as raw bytes.
    Binary(Vec<u8>),
}

/// Where daemon → webview frames go: a Tauri `Channel` in the app, a plain closure
/// under test. The indirection is what lets a connection's lifetime be exercised
/// without standing up a webview.
pub trait FrameSink: Send + Sync + 'static {
    fn send(&self, frame: Frame);
}

impl FrameSink for Channel<InvokeResponseBody> {
    fn send(&self, frame: Frame) {
        let body = match frame {
            // A JSON string literal: the webview receives it as a JS string, exactly
            // what a `Channel<String>` delivered before binary frames existed.
            Frame::Text(text) => match serde_json::to_string(&text) {
                Ok(json) => InvokeResponseBody::Json(json),
                Err(_) => return,
            },
            // Raw bytes: the webview receives an ArrayBuffer, with no JSON escaping.
            Frame::Binary(bytes) => InvokeResponseBody::Raw(bytes),
        };
        // Errors here mean the webview is gone, i.e. the app is shutting down. They
        // say nothing about the connection (in particular, a page reload does NOT
        // produce one), so there is nothing useful to do but drop the frame.
        let _ = Channel::send(self, body);
    }
}

/// Open a WebSocket to the daemon for `path` (a full `/v0/...` route) and start
/// streaming its frames to `on_message`. Each frame is forwarded RAW — the whole
/// DTO text, or the binary payload — because the frontend parses it itself.
///
/// `host_id` names a paired remote daemon, routed by the same rule as `crowbar://`
/// requests; absent, empty or `localhost` is the local one.
///
/// `idle_timeout_ms` arms a read-idle timeout for a stream whose daemon side pings
/// (the terminal's, every 45s): a socket that delivers nothing at all — not a frame,
/// not a ping — for that long is half-open, and is closed and announced like any
/// other daemon-side close rather than parked on forever.
#[tauri::command]
pub async fn ws_open(
    conn_id: String,
    path: String,
    host_id: Option<String>,
    on_message: Channel<InvokeResponseBody>,
    idle_timeout_ms: Option<u64>,
    manager: State<'_, WsBridgeManager>,
    window: tauri::WebviewWindow,
) -> Result<(), String> {
    let window_label = window.label().to_string();
    let read_idle_timeout = idle_timeout_ms.map(Duration::from_millis);
    let opened = match remote_host(host_id) {
        None => {
            let socket = crate::sidecar::socket_path();
            open_bridge(
                &socket,
                conn_id,
                path,
                on_message,
                &manager,
                window_label,
                read_idle_timeout,
            )
            .await
        }
        Some(host_id) => {
            let registry = window.state::<HostRegistry>();
            let tokens = window.state::<Arc<dyn TokenStore>>();
            let paired = remote::paired_host(&registry, &tokens, &host_id).await?;
            remote::open_remote_bridge(
                paired,
                conn_id,
                path,
                on_message,
                &manager,
                window_label,
                read_idle_timeout,
            )
            .await
        }
    };
    opened.map(|_reader| ())
}

fn remote_host(host_id: Option<String>) -> Option<String> {
    host_id.filter(|host| crate::api_proxy::is_remote_host(host))
}

/// The transport half of [`ws_open`], free of Tauri's `State` so it can be driven
/// directly against a real unix-socket server.
///
/// Returns the reader task's handle. The app ignores it — the connection outlives the
/// call — but it is the only honest signal that a retired connection has finished
/// letting go of its socket, so tests block on it rather than on a peer that may never
/// react (which is exactly the case cancellation exists for).
pub async fn open_bridge<S: FrameSink>(
    socket: &Path,
    conn_id: String,
    path: String,
    on_message: S,
    manager: &WsBridgeManager,
    window_label: String,
    read_idle_timeout: Option<Duration>,
) -> Result<tokio::task::JoinHandle<()>, String> {
    let stream = UnixStream::connect(socket).await.map_err(|e| {
        log::error!("ws_open: connect daemon socket failed: {e}");
        format!("connect daemon socket: {e}")
    })?;
    let ws = upgrade(stream, upgrade_request(&path)?).await?;
    Ok(attach(
        ws,
        conn_id,
        on_message,
        manager,
        window_label,
        read_idle_timeout,
    ))
}

/// The upgrade request for `path` (already `/v0/...`). The URL's host is irrelevant —
/// the transport is the stream it is sent over — but tungstenite needs it to build the
/// handshake's request line and Host.
fn upgrade_request(path: &str) -> Result<Request, String> {
    format!("ws://localhost{path}")
        .into_client_request()
        .map_err(|e| format!("build ws request: {e}"))
}

async fn upgrade<T>(stream: T, request: Request) -> Result<WebSocketStream<T>, String>
where
    T: AsyncRead + AsyncWrite + Unpin,
{
    let (ws, _resp) = client_async(request, stream).await.map_err(|e| {
        log::error!("ws_open: ws upgrade failed: {e}");
        format!("ws upgrade: {e}")
    })?;
    Ok(ws)
}

/// Send a raw text frame (the JSON the frontend wants to publish) to the daemon.
#[tauri::command]
pub async fn ws_send(
    conn_id: String,
    data: String,
    manager: State<'_, WsBridgeManager>,
) -> Result<(), String> {
    match manager.sender(&conn_id) {
        Some(tx) => tx
            .send(Message::Text(data.into()))
            .map_err(|_| format!("ws connection {conn_id} is closed")),
        None => Err(format!("no open ws connection {conn_id}")),
    }
}

/// Close the WebSocket leg for a connection.
#[tauri::command]
pub async fn ws_close(conn_id: String, manager: State<'_, WsBridgeManager>) -> Result<(), String> {
    manager.connections.lock().unwrap().remove(&conn_id);
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_a_non_localhost_host_id_opens_a_remote_bridge() {
        assert_eq!(remote_host(None), None);
        assert_eq!(remote_host(Some(String::new())), None);
        assert_eq!(remote_host(Some("localhost".into())), None);
        assert_eq!(remote_host(Some("host-1".into())), Some("host-1".into()));
    }
}
