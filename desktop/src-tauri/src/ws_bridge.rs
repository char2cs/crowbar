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
//!
//! # Connection lifetime
//!
//! A connection's unix socket stays open while *either* split half is alive, so
//! both the reader and the writer task must finish before its fd is returned.
//! Both close directions therefore funnel through one retirement path in the
//! reader task (see `open_bridge`). This is load-bearing, not tidiness: the app
//! runs at macOS's default soft limit of 256 fds (Go raises its own; Rust does
//! not), and the daemon closes these streams constantly — every watcher release
//! and workspace switch. A connection left half-alive burns an fd for the life
//! of the process.

use std::collections::HashMap;
use std::path::Path;
use std::sync::{Arc, Mutex};

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

/// Where daemon → webview frames go: a Tauri `Channel` in the app, a plain
/// closure under test. The indirection is what lets a connection's lifetime be
/// exercised without standing up a webview.
pub trait FrameSink: Send + Sync + 'static {
    /// Delivers one frame. Returns false once the receiver is gone — the webview
    /// navigated away and the connection has no reader left.
    fn send(&self, frame: String) -> bool;
}

impl FrameSink for Channel<String> {
    fn send(&self, frame: String) -> bool {
        Channel::send(self, frame).is_ok()
    }
}

type Connections = Arc<Mutex<HashMap<String, mpsc::UnboundedSender<Message>>>>;

/// Maps a connection id to the sender feeding its WebSocket writer task. One
/// writer task per connection serialises all outbound frames so command handlers
/// never touch the socket directly.
#[derive(Default)]
pub struct WsBridgeManager {
    connections: Connections,
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
    open_bridge(&socket, conn_id, path, on_message, &manager).await
}

/// The transport half of [`ws_open`], free of Tauri's `State`/`Channel` so it can
/// be driven directly against a real unix-socket server.
pub async fn open_bridge<S: FrameSink>(
    socket: &Path,
    conn_id: String,
    path: String,
    on_message: S,
    manager: &WsBridgeManager,
) -> Result<(), String> {
    let stream = UnixStream::connect(socket).await.map_err(|e| {
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
    // It ends when every sender is dropped, which is what retiring the
    // connection below does.
    let (tx, mut rx) = mpsc::unbounded_channel::<Message>();
    let writer = tokio::spawn(async move {
        while let Some(msg) = rx.recv().await {
            if write.send(msg).await.is_err() {
                break;
            }
        }
        let _ = write.close().await;
    });

    // A re-used id would otherwise strand the previous connection's writer.
    if let Some(stale) = manager
        .connections
        .lock()
        .unwrap()
        .insert(conn_id.clone(), tx)
    {
        drop(stale);
    }

    // Reader task: forward each text frame verbatim to the webview channel, then
    // retire the connection on any close/error/EOF and tell the shim, so it
    // fires `onclose` and `wsManager` reconnects + re-seeds (§6 reconnect
    // recovery on desktop).
    let connections = Arc::clone(&manager.connections);
    tokio::spawn(async move {
        let mut sink_alive = true;
        while let Some(frame) = read.next().await {
            match frame {
                Ok(Message::Text(text)) => {
                    if !on_message.send(text) {
                        sink_alive = false;
                        break;
                    }
                }
                Ok(Message::Close(_)) | Err(_) => break,
                _ => {}
            }
        }
        drop(read);

        // The one retirement path, shared by both close directions. Dropping the
        // sender is what lets the writer task finish; awaiting it is what
        // guarantees the socket's other half is gone. Only then is the fd back —
        // and only then does the shim learn it may reconnect, so a reconnect can
        // never race ahead of the fd it is about to need.
        connections.lock().unwrap().remove(&conn_id);
        let _ = writer.await;

        if sink_alive {
            on_message.send(WS_CLOSE_SENTINEL.to_string());
        }
    });

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

#[cfg(test)]
mod tests {
    use super::*;
    use tokio::net::UnixListener;
    use tokio::sync::oneshot;

    /// File descriptors this process holds open. macOS exposes them as /dev/fd.
    fn open_fds() -> usize {
        std::fs::read_dir("/dev/fd").map(|d| d.count()).unwrap_or(0)
    }

    /// Short path: a unix socket's sun_path is capped at 104 bytes and $TMPDIR
    /// on macOS is long enough to matter.
    fn test_socket(tag: &str) -> std::path::PathBuf {
        let p = std::path::PathBuf::from(format!("/tmp/cbws-{}-{tag}.sock", std::process::id()));
        let _ = std::fs::remove_file(&p);
        p
    }

    struct ClosureSink<F>(F);

    impl<F: Fn(String) -> bool + Send + Sync + 'static> FrameSink for ClosureSink<F> {
        fn send(&self, frame: String) -> bool {
            (self.0)(frame)
        }
    }

    /// Stands in for the daemon closing a stream on its own initiative — what it
    /// does on every watcher release and workspace switch, and what the
    /// `files/ws` churn in a pre-crash daemon log is made of.
    async fn accept_then_close(listener: &UnixListener) {
        if let Ok((stream, _)) = listener.accept().await {
            if let Ok(ws) = tokio_tungstenite::accept_async(stream).await {
                drop(ws);
            }
        }
    }

    /// A sink that fires `tx` the moment the close sentinel arrives, so tests can
    /// block on the connection actually being torn down rather than on a clock.
    fn sentinel_sink(tx: oneshot::Sender<()>) -> impl FrameSink {
        let tx = Mutex::new(Some(tx));
        ClosureSink(move |frame: String| {
            if frame == WS_CLOSE_SENTINEL {
                if let Some(tx) = tx.lock().unwrap().take() {
                    let _ = tx.send(());
                }
            }
            true
        })
    }

    #[tokio::test]
    async fn daemon_initiated_close_retires_the_connection() {
        let sock = test_socket("retire");
        let listener = UnixListener::bind(&sock).unwrap();
        let manager = WsBridgeManager::new();

        let (tx, rx) = oneshot::channel();
        let opened = open_bridge(
            &sock,
            "c1".to_string(),
            "/v0/x".to_string(),
            sentinel_sink(tx),
            &manager,
        );
        let (opened, _) = tokio::join!(opened, accept_then_close(&listener));
        opened.unwrap();

        rx.await.expect("the shim must be told the daemon closed");

        assert!(
            manager.connections.lock().unwrap().is_empty(),
            "the daemon closed the stream, so the connection must be retired; a \
             sender left in the map strands its writer task on the socket's write \
             half and burns one of the app's 256 fds for good"
        );

        let _ = std::fs::remove_file(&sock);
    }

    #[tokio::test]
    async fn daemon_initiated_closes_do_not_leak_file_descriptors() {
        const CYCLES: usize = 20;

        let sock = test_socket("fds");
        let listener = UnixListener::bind(&sock).unwrap();
        let manager = WsBridgeManager::new();
        let mut steady_state = 0;

        // Every cycle is one reconnect: the shim opens a fresh conn_id, the daemon
        // closes it. This is the loop a pre-crash daemon log shows hundreds of.
        for i in 0..CYCLES {
            let (tx, rx) = oneshot::channel();
            let opened = open_bridge(
                &sock,
                format!("c{i}"),
                "/v0/x".to_string(),
                sentinel_sink(tx),
                &manager,
            );
            let (opened, _) = tokio::join!(opened, accept_then_close(&listener));
            opened.unwrap();
            rx.await.unwrap();

            // After the first cycle the runtime's own fds are all allocated, so
            // this is the level a leak-free bridge must return to every time.
            if i == 0 {
                steady_state = open_fds();
            }
        }

        let after = open_fds();
        assert!(
            after <= steady_state,
            "{CYCLES} daemon-initiated closes leaked file descriptors: {steady_state} -> {after}. \
             The app's soft limit is 256, and once it is reached the daemon socket \
             cannot be dialled at all — which the health watchdog reads as a dead \
             backend and kills a daemon that was never unhealthy."
        );

        let _ = std::fs::remove_file(&sock);
    }
}
