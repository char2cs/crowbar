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
//!   * webview → daemon: `terminal_send` / `terminal_resize` / `terminal_resync` /
//!     `terminal_set_theme` enqueue `{data}` / `{type:"resize",cols,rows}` /
//!     `{type:"resync"}` / `{type:"theme",bg,fg,dark}` frames for the session's writer.
//!
//! The frontend still creates the PTY session with a normal `POST` over the
//! proxy; only the streaming leg comes through these commands.

use std::collections::HashMap;
use std::sync::Mutex;

use futures_util::{SinkExt, StreamExt};
use tauri::ipc::Channel;
use tauri::{Emitter, Manager, State};
use tokio::net::UnixStream;
use tokio::sync::{mpsc, oneshot};
use tokio_tungstenite::client_async;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;
use tokio_tungstenite::tungstenite::Message;

/// Everything a live streaming leg needs in order to die. Dropping it ends both of
/// the session's tasks, and so releases its socket: the sender going away ends the
/// writer, and the cancel channel going away ends the reader. See the lifetime notes
/// on `ws_bridge` — a socket survives while *either* split half does, and the reader
/// otherwise parks on `read.next()` forever against a daemon that keeps the PTY open.
struct Streaming {
    generation: u64,
    tx: mpsc::UnboundedSender<Message>,
    /// Held, never used. Its `Drop` is the signal.
    _cancel: oneshot::Sender<()>,
}

/// Maps a PTY session id to its live streaming leg. One writer task per session
/// serialises all outbound frames so command handlers never touch the socket.
///
/// Each open is stamped with a GENERATION: Rust outlives the webview, so after a page
/// reload the PRE-reload reader task must not emit `terminal:transport-dropped` for a
/// session the POST-reload webview has already re-opened. Only the reader whose
/// generation is still current may emit it; a stale reader exits silently. Re-opening
/// also drops the previous entry, which retires it in full — the daemon keeps the PTY
/// alive, so the reloaded page simply re-attaches.
#[derive(Default)]
pub struct TerminalManager {
    sessions: Mutex<HashMap<String, Streaming>>,
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
            .map(|s| s.tx.clone())
    }

    fn register(
        &self,
        session_id: String,
        tx: mpsc::UnboundedSender<Message>,
        cancel: oneshot::Sender<()>,
    ) -> u64 {
        let generation = self
            .next_generation
            .fetch_add(1, std::sync::atomic::Ordering::Relaxed);
        // Dropping a previous entry retires it in full: its writer ends, its reader is
        // cancelled, and the old WS closes so the daemon detaches the stale client.
        self.sessions.lock().unwrap().insert(
            session_id,
            Streaming {
                generation,
                tx,
                _cancel: cancel,
            },
        );
        generation
    }

    /// Retires `session_id`, but only while `generation` is still the current one — a
    /// stale pre-reload reader must not evict the transport a reloaded webview has
    /// since opened. Returns whether it retired.
    fn retire_if_current(&self, session_id: &str, generation: u64) -> bool {
        let mut sessions = self.sessions.lock().unwrap();
        match sessions.get(session_id) {
            Some(s) if s.generation == generation => {
                sessions.remove(session_id);
                true
            }
            _ => false,
        }
    }

    /// Retires every streaming leg. Called when a page load orphans them all: the JS
    /// that owned these sessions is gone and will never close them, and a reloaded page
    /// re-attaches the ones it still wants. The daemon keeps each PTY alive across this
    /// — only the streaming leg is torn down, never the session.
    pub fn close_all(&self) {
        self.sessions.lock().unwrap().clear();
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
    // It ends when every sender is dropped — which is what retiring the session
    // does, and the only way the socket's write half is ever handed back.
    let (tx, mut rx) = mpsc::unbounded_channel::<Message>();
    let writer = tokio::spawn(async move {
        while let Some(msg) = rx.recv().await {
            if write.send(msg).await.is_err() {
                break;
            }
        }
        let _ = write.close().await;
    });

    // Register FIRST so the reader task can check its own generation on exit.
    let (cancel, mut cancelled) = oneshot::channel();
    let generation = manager.register(session_id.clone(), tx, cancel);

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
        loop {
            tokio::select! {
                // The session was retired: closed, re-opened under a new generation, or
                // orphaned by a page load. Whoever removed it has already retired it —
                // just let go of the read half so the socket can close. A daemon that
                // keeps the PTY open sends nothing, so without this the reader would
                // park here for the life of the app, holding the socket.
                _ = &mut cancelled => break,
                frame = read.next() => match frame {
                    // An error here means the webview is gone (app shutdown); it does
                    // NOT mean the page reloaded, so it is not a teardown signal.
                    Some(Ok(Message::Text(text))) => {
                        let _ = on_data.send(text.to_string());
                    }
                    Some(Ok(_)) => continue,
                    Some(Err(_)) | None => break,
                },
            }
        }
        drop(read);

        // Retire the session before announcing the drop. Dropping its entry ends the
        // writer task, and awaiting the writer is what guarantees the socket's other
        // half is gone — until both halves are, the descriptor stays open. A stale
        // generation was already evicted by whoever superseded it, so its writer is
        // ending on its own: await it, but leave the live session alone.
        let still_current = app
            .state::<TerminalManager>()
            .retire_if_current(&session_id, generation);
        let _ = writer.await;

        // Only an unexpected daemon disconnect is worth announcing. A retired session
        // was closed, superseded, or orphaned by a page load — in each case whoever
        // retired it already knows, and a reloaded page must not be told a session it
        // is about to re-attach has "dropped".
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

/// Push the host light/dark theme to the daemon so a foreground app's automatic theme
/// (Claude Code's `auto`) can follow a Crowbar theme switch: `bg`/`fg` are the resolved
/// default colours an OSC 11/10 query answers with, `dark` the light/dark polarity for the
/// daemon's DEC 2031 CSI ?997;n theme-change report.
#[tauri::command]
pub async fn terminal_set_theme(
    session_id: String,
    bg: String,
    fg: String,
    dark: bool,
    manager: State<'_, TerminalManager>,
) -> Result<(), String> {
    let frame =
        serde_json::json!({ "type": "theme", "bg": bg, "fg": fg, "dark": dark }).to_string();
    enqueue(&manager, &session_id, Message::Text(frame))
}

/// Close the WebSocket leg for a session. The daemon-side PTY is torn down
/// separately by the frontend's `DELETE .../terminals/:id`.
#[tauri::command]
pub async fn terminal_close(
    session_id: String,
    manager: State<'_, TerminalManager>,
) -> Result<(), String> {
    let retired = manager.sessions.lock().unwrap().remove(&session_id);
    if let Some(streaming) = retired {
        // Best-effort close frame. Dropping the entry after it is queued is what ends
        // both tasks and closes the socket, so a daemon that never answers the frame
        // cannot keep the descriptor.
        let _ = streaming.tx.send(Message::Close(None));
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
