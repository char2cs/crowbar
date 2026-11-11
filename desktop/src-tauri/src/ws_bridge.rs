//! Bridges `/v0/...` WebSocket routes between the webview and the daemon. The webview
//! cannot dial the daemon's unix socket itself (`WebSocket` only takes `ws`/`wss`), so Rust
//! is the client; text frames reach JS as strings, binary (PTY output) as ArrayBuffers.

#[cfg(desktop)]
use std::path::Path;
use std::sync::Arc;
use std::time::Duration;

use tauri::ipc::{Channel, InvokeResponseBody};
use tauri::{Manager, State};
use tokio::io::{AsyncRead, AsyncWrite};
#[cfg(desktop)]
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

/// Pushed down the Channel when the daemon closes the stream, so the JS shim fires
/// `onclose` and reconnects. The NUL prefix cannot collide with a JSON frame.
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

/// Opens a WebSocket to the daemon (local, or the paired `host_id`) for `path` and streams
/// its frames raw to `on_message`. `idle_timeout_ms` is for streams whose daemon pings: a
/// socket silent that long is half-open, so it is closed and announced like a daemon close.
#[tauri::command]
pub async fn ws_open(
    conn_id: String,
    path: String,
    host_id: Option<String>,
    on_message: Channel<InvokeResponseBody>,
    idle_timeout_ms: Option<u64>,
    manager: State<'_, WsBridgeManager>,
    window: tauri::Window,
) -> Result<(), String> {
    let window_label = window.label().to_string();
    let read_idle_timeout = idle_timeout_ms.map(Duration::from_millis);
    let opened = match remote_host(host_id) {
        #[cfg(desktop)]
        None => {
            let socket =
                crate::sidecar::socket_path().ok_or_else(|| "daemon not started".to_string())?;
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
        #[cfg(mobile)]
        None => return Err("no local daemon on this device".to_string()),
        Some(host_id) => {
            let registry = window.state::<HostRegistry>();
            let tokens = window.state::<Arc<dyn TokenStore>>();
            let paired = remote::paired_host(&registry, &tokens, &host_id)
                .await
                .inspect_err(|e| log::error!("ws_open: remote host {host_id}: {e}"))?;
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

/// The transport half of [`ws_open`], free of Tauri's `State`. Returns the reader task,
/// whose completion is the only honest signal that a retired connection released its
/// socket; tests wait on it rather than on a peer that may never react.
#[cfg(desktop)]
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
        None,
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
