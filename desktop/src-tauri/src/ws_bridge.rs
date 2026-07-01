//! Generic WebSocket-over-unix-socket bridge for the desktop app.
//!
//! The webview cannot open a `WebSocket` to the daemon: its only reachable
//! endpoint is the `crowbar://` unix-socket proxy (see `api_proxy`), and the
//! browser `WebSocket` constructor rejects every scheme but `ws`/`wss`. So Rust
//! is the WebSocket client here — it dials the daemon's existing unix socket
//! (where a WS upgrade is perfectly legal) and bridges an arbitrary `/v0/...`
//! WebSocket route both ways to the webview.
//!
//! This generalises the terminal PTY bridge (`terminal.rs`) for the §6 live
//! entity cache: each daemon → webview frame is forwarded RAW (the whole JSON
//! DTO text), not just a `.data` field, because the wire frames are now complete
//! DTOs the `wsManager` parses itself.
//!
//!   * daemon → webview: each `Message::Text(text)` is pushed verbatim down a
//!     Tauri `Channel<String>` the frontend supplies at open time.
//!   * webview → daemon: `ws_send` enqueues the supplied text frame for the
//!     connection's writer task.
//!
//! Connections are keyed by a client-supplied id (`crypto.randomUUID` on the JS
//! side) so multiple scoped streams can coexist.

use std::collections::HashMap;
use std::sync::Mutex;

use futures_util::{SinkExt, StreamExt};
use tauri::ipc::Channel;
use tauri::State;
use tokio::net::UnixStream;
use tokio::sync::mpsc;
use tokio_tungstenite::client_async;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;
use tokio_tungstenite::tungstenite::Message;

/// Sentinel pushed down the message Channel when the daemon closes the stream
/// (close frame, error, or EOF). The JS shim recognises it and fires `onclose`
/// so `wsManager` reconnects and the §6 cache re-seeds. A NUL prefix makes it
/// impossible to collide with a real JSON DTO frame (which always starts `{`).
pub const WS_CLOSE_SENTINEL: &str = "\u{0}crowbar-ws-close";

/// Maps a connection id to the sender feeding its WebSocket writer task. One
/// writer task per connection serialises all outbound frames so command handlers
/// never touch the socket directly.
#[derive(Default)]
pub struct WsBridgeManager {
    connections: Mutex<HashMap<String, mpsc::UnboundedSender<Message>>>,
}

impl WsBridgeManager {
    pub fn new() -> Self {
        Self::default()
    }

    fn sender(&self, conn_id: &str) -> Option<mpsc::UnboundedSender<Message>> {
        self.connections.lock().unwrap().get(conn_id).cloned()
    }
}

/// Open a WebSocket to the daemon for `path` (a full `/v0/...` route) and start
/// streaming its frames to `on_message`. Each text frame is forwarded RAW — the
/// whole DTO object — because the frontend parses the complete frame itself.
#[tauri::command]
pub async fn ws_open(
    conn_id: String,
    path: String,
    on_message: Channel<String>,
    manager: State<'_, WsBridgeManager>,
) -> Result<(), String> {
    let socket = crate::sidecar::socket_path();
    let stream = UnixStream::connect(&socket).await.map_err(|e| {
        log::error!("ws_open: connect daemon socket failed: {e}");
        format!("connect daemon socket: {e}")
    })?;

    // The URL's host is irrelevant — the transport is the UnixStream we hand in —
    // but tungstenite needs it to build the handshake's request line and Host.
    // `path` already starts with /v0/...
    let request = format!("ws://localhost{path}")
        .into_client_request()
        .map_err(|e| format!("build ws request: {e}"))?;
    let (ws, _resp) = client_async(request, stream).await.map_err(|e| {
        log::error!("ws_open: ws upgrade failed: {e}");
        format!("ws upgrade: {e}")
    })?;
    let (mut write, mut read) = ws.split();

    // Writer task: drain the mpsc and push frames at the socket one at a time.
    let (tx, mut rx) = mpsc::unbounded_channel::<Message>();
    tokio::spawn(async move {
        while let Some(msg) = rx.recv().await {
            if write.send(msg).await.is_err() {
                break;
            }
        }
        let _ = write.close().await;
    });

    // Reader task: forward each text frame verbatim to the webview channel. On
    // any close/error/EOF, push the close sentinel so the JS shim fires onclose
    // and the manager can reconnect + re-seed (§6 reconnect recovery on desktop).
    tokio::spawn(async move {
        while let Some(frame) = read.next().await {
            match frame {
                Ok(Message::Text(text)) => {
                    if on_message.send(text).is_err() {
                        return;
                    }
                }
                Ok(Message::Close(_)) | Err(_) => break,
                _ => {}
            }
        }
        let _ = on_message.send(WS_CLOSE_SENTINEL.to_string());
    });

    manager.connections.lock().unwrap().insert(conn_id, tx);
    Ok(())
}

/// Send a raw text frame (the JSON the frontend wants to publish) to the daemon.
#[tauri::command]
pub async fn ws_send(
    conn_id: String,
    data: String,
    manager: State<'_, WsBridgeManager>,
) -> Result<(), String> {
    enqueue(&manager, &conn_id, Message::Text(data))
}

/// Close the WebSocket leg for a connection.
#[tauri::command]
pub async fn ws_close(conn_id: String, manager: State<'_, WsBridgeManager>) -> Result<(), String> {
    let sender = manager.connections.lock().unwrap().remove(&conn_id);
    if let Some(tx) = sender {
        // Best-effort close frame; dropping the sender ends the writer task.
        let _ = tx.send(Message::Close(None));
    }
    Ok(())
}

fn enqueue(manager: &WsBridgeManager, conn_id: &str, msg: Message) -> Result<(), String> {
    match manager.sender(conn_id) {
        Some(tx) => tx
            .send(msg)
            .map_err(|_| format!("ws connection {conn_id} is closed")),
        None => Err(format!("no open ws connection {conn_id}")),
    }
}
