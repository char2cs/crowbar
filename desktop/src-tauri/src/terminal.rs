//! Terminal PTY transport for the desktop app.
//!
//! The webview cannot open a `WebSocket` to the daemon: its only reachable
//! endpoint is the `crowbar://` unix-socket proxy (see `api_proxy`), and the
//! browser `WebSocket` constructor rejects every scheme but `ws`/`wss`. So Rust
//! is the WebSocket client here — it dials the daemon's existing
//! `/v0/ws/terminals/:id` route over the same unix socket (where a WS upgrade is
//! perfectly legal) and bridges the PTY both ways to the webview:
//!
//!   * daemon → webview: each `{sessionId, data, isInput}` frame's `data` is
//!     pushed down a Tauri `Channel<String>` the frontend supplies at open time.
//!   * webview → daemon: `terminal_send` / `terminal_resize` enqueue
//!     `{data}` / `{type:"resize",cols,rows}` frames for the session's writer.
//!
//! The frontend still creates the PTY session with a normal `POST` over the
//! proxy; only the streaming leg comes through these commands.

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

/// Maps a PTY session id to the sender feeding its WebSocket writer task. One
/// writer task per session serialises all outbound frames so command handlers
/// never touch the socket directly.
#[derive(Default)]
pub struct TerminalManager {
    sessions: Mutex<HashMap<String, mpsc::UnboundedSender<Message>>>,
}

impl TerminalManager {
    pub fn new() -> Self {
        Self::default()
    }

    fn sender(&self, session_id: &str) -> Option<mpsc::UnboundedSender<Message>> {
        self.sessions.lock().unwrap().get(session_id).cloned()
    }
}

/// Open the WebSocket for an already-created PTY session and start streaming its
/// output to `on_data`. The session id comes from the `POST .../terminals` the
/// frontend already issued over the proxy.
#[tauri::command]
pub async fn terminal_open(
    session_id: String,
    on_data: Channel<String>,
    manager: State<'_, TerminalManager>,
) -> Result<(), String> {
    let socket = crate::sidecar::socket_path();
    let stream = UnixStream::connect(&socket).await.map_err(|e| {
        log::error!("terminal_open: connect daemon socket failed: {e}");
        format!("connect daemon socket: {e}")
    })?;

    // The URL's host is irrelevant — the transport is the UnixStream we hand in —
    // but tungstenite needs it to build the handshake's request line and Host.
    let request = format!("ws://localhost/v0/ws/terminals/{session_id}")
        .into_client_request()
        .map_err(|e| format!("build ws request: {e}"))?;
    let (ws, _resp) = client_async(request, stream).await.map_err(|e| {
        log::error!("terminal_open: ws upgrade failed: {e}");
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

    // Reader task: forward each frame's `data` payload to the webview channel.
    tokio::spawn(async move {
        while let Some(frame) = read.next().await {
            match frame {
                Ok(Message::Text(text)) => {
                    if let Ok(value) = serde_json::from_str::<serde_json::Value>(&text) {
                        if let Some(data) = value.get("data").and_then(|d| d.as_str()) {
                            if on_data.send(data.to_string()).is_err() {
                                break;
                            }
                        }
                    }
                }
                Ok(Message::Close(_)) | Err(_) => break,
                _ => {}
            }
        }
    });

    manager.sessions.lock().unwrap().insert(session_id, tx);
    Ok(())
}

/// Send user input to the PTY.
#[tauri::command]
pub async fn terminal_send(
    session_id: String,
    data: String,
    manager: State<'_, TerminalManager>,
) -> Result<(), String> {
    let frame = serde_json::json!({ "data": data }).to_string();
    enqueue(&manager, &session_id, Message::Text(frame))
}

/// Resize the PTY (SIGWINCH).
#[tauri::command]
pub async fn terminal_resize(
    session_id: String,
    rows: u16,
    cols: u16,
    manager: State<'_, TerminalManager>,
) -> Result<(), String> {
    let frame = serde_json::json!({ "type": "resize", "cols": cols, "rows": rows }).to_string();
    enqueue(&manager, &session_id, Message::Text(frame))
}

/// Close the WebSocket leg for a session. The daemon-side PTY is torn down
/// separately by the frontend's `DELETE .../terminals/:id`.
#[tauri::command]
pub async fn terminal_close(
    session_id: String,
    manager: State<'_, TerminalManager>,
) -> Result<(), String> {
    let sender = manager.sessions.lock().unwrap().remove(&session_id);
    if let Some(tx) = sender {
        // Best-effort close frame; dropping the sender ends the writer task.
        let _ = tx.send(Message::Close(None));
    }
    Ok(())
}

fn enqueue(
    manager: &TerminalManager,
    session_id: &str,
    msg: Message,
) -> Result<(), String> {
    match manager.sender(session_id) {
        Some(tx) => tx
            .send(msg)
            .map_err(|_| format!("terminal session {session_id} is closed")),
        None => Err(format!("no open terminal session {session_id}")),
    }
}
