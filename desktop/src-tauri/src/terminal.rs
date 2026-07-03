//! Terminal PTY transport for the desktop app.
//!
//! The webview cannot open a `WebSocket` to the daemon: its only reachable
//! endpoint is the `crowbar://` unix-socket proxy (see `api_proxy`), and the
//! browser `WebSocket` constructor rejects every scheme but `ws`/`wss`. So Rust
//! is the WebSocket client here — it dials the daemon's existing
//! `/v0/ws/terminals/:id` route over the same unix socket (where a WS upgrade is
//! perfectly legal) and bridges the PTY both ways to the webview:
//!
//!   * daemon → webview: each wire frame (`{sessionId, data, isInput, snapshot?}`)
//!     is forwarded WHOLE down a Tauri `Channel<String>`; the frontend bridge
//!     parses it (same as the browser-WebSocket path) so frame semantics like
//!     the snapshot flag live in one place, not here.
//!   * webview → daemon: `terminal_send` / `terminal_resize` / `terminal_resync`
//!     enqueue `{data}` / `{type:"resize",cols,rows}` / `{type:"resync"}` frames
//!     for the session's writer.
//!
//! The frontend still creates the PTY session with a normal `POST` over the
//! proxy; only the streaming leg comes through these commands.

use std::collections::HashMap;
use std::sync::Mutex;

use futures_util::{SinkExt, StreamExt};
use tauri::ipc::Channel;
use tauri::{Emitter, Manager, State};
use tokio::net::UnixStream;
use tokio::sync::mpsc;
use tokio_tungstenite::client_async;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;
use tokio_tungstenite::tungstenite::Message;

/// Maps a PTY session id to the sender feeding its WebSocket writer task. One
/// writer task per session serialises all outbound frames so command handlers
/// never touch the socket directly.
///
/// Each open is stamped with a GENERATION: Rust outlives the webview, so after
/// a page reload the PRE-reload reader task (whose channel points into the dead
/// webview) eventually exits — and must not emit `terminal:transport-dropped`
/// for a session the POST-reload webview has already re-opened. Only the reader
/// whose generation is still current may emit the drop event; a stale reader
/// exits silently. Re-opening also drops the previous sender, ending the old
/// writer task and closing the old WS so the daemon detaches the stale client.
#[derive(Default)]
pub struct TerminalManager {
    sessions: Mutex<HashMap<String, (u64, mpsc::UnboundedSender<Message>)>>,
    next_generation: std::sync::atomic::AtomicU64,
}

impl TerminalManager {
    pub fn new() -> Self {
        Self::default()
    }

    fn sender(&self, session_id: &str) -> Option<mpsc::UnboundedSender<Message>> {
        self.sessions
            .lock()
            .unwrap()
            .get(session_id)
            .map(|(_, tx)| tx.clone())
    }

    fn register(&self, session_id: String, tx: mpsc::UnboundedSender<Message>) -> u64 {
        let generation = self
            .next_generation
            .fetch_add(1, std::sync::atomic::Ordering::Relaxed);
        // Dropping a previous entry's sender ends its writer task → old WS closes.
        self.sessions
            .lock()
            .unwrap()
            .insert(session_id, (generation, tx));
        generation
    }

    fn is_current(&self, session_id: &str, generation: u64) -> bool {
        self.sessions
            .lock()
            .unwrap()
            .get(session_id)
            .map(|(g, _)| *g == generation)
            .unwrap_or(false)
    }
}

/// Open the WebSocket for an already-created PTY session and start streaming its
/// output to `on_data`. The session id comes from the `POST .../terminals` the
/// frontend already issued over the proxy.
#[tauri::command]
pub async fn terminal_open(
    session_id: String,
    // §3: the hierarchical PTY WS path, e.g.
    // /v0/projects/:p/repos/:r/workspaces/:w/terminals/:sessionId/ws. The
    // frontend builds it (workspace-scope aware) and hands it down so Rust no
    // longer hardcodes the removed flat /v0/ws/terminals/:id route.
    ws_path: String,
    on_data: Channel<String>,
    manager: State<'_, TerminalManager>,
    app: tauri::AppHandle,
) -> Result<(), String> {
    let socket = crate::sidecar::socket_path();
    let stream = UnixStream::connect(&socket).await.map_err(|e| {
        log::error!("terminal_open: connect daemon socket failed: {e}");
        format!("connect daemon socket: {e}")
    })?;

    // The URL's host is irrelevant — the transport is the UnixStream we hand in —
    // but tungstenite needs it to build the handshake's request line and Host.
    let request = format!("ws://localhost{ws_path}")
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

    // Register FIRST so the reader task can check its own generation on exit.
    let generation = manager.register(session_id.clone(), tx);

    // Reader task: forward each text frame WHOLE to the webview channel — the
    // TS bridge parses `{data, snapshot}` for both transports in one place.
    // After the loop exits, emit `terminal:transport-dropped` so the JS side
    // can detect an unexpected daemon disconnect and trigger reconnect — but
    // ONLY if this reader is still the session's current generation: a stale
    // pre-reload reader dying must not tear down the fresh transport the
    // reloaded webview just opened (the "terminal silent after reload" bug).
    // The JS guard (`tauriTerminals.has(connectionId)`) additionally
    // distinguishes unexpected drops from clean terminal_close paths.
    tokio::spawn(async move {
        while let Some(frame) = read.next().await {
            match frame {
                Ok(Message::Text(text)) => {
                    if on_data.send(text.to_string()).is_err() {
                        break;
                    }
                }
                Ok(Message::Close(_)) | Err(_) => break,
                _ => {}
            }
        }
        let still_current = app
            .state::<TerminalManager>()
            .is_current(&session_id, generation);
        if still_current {
            let _ = app.emit("terminal:transport-dropped", session_id);
        }
    });

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

/// Ask the daemon to re-emit the model snapshot (post-resize convergence).
#[tauri::command]
pub async fn terminal_resync(
    session_id: String,
    manager: State<'_, TerminalManager>,
) -> Result<(), String> {
    let frame = serde_json::json!({ "type": "resync" }).to_string();
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
    if let Some((_, tx)) = sender {
        // Best-effort close frame; dropping the sender ends the writer task.
        let _ = tx.send(Message::Close(None));
    }
    Ok(())
}

fn enqueue(manager: &TerminalManager, session_id: &str, msg: Message) -> Result<(), String> {
    match manager.sender(session_id) {
        Some(tx) => tx
            .send(msg)
            .map_err(|_| format!("terminal session {session_id} is closed")),
        None => Err(format!("no open terminal session {session_id}")),
    }
}
