//! Generic WebSocket-over-unix-socket bridge for the desktop app.
//!
//! The webview cannot open a `WebSocket` to the daemon: its only reachable
//! endpoint is the `crowbar://` unix-socket proxy (see `api_proxy`), and the
//! browser `WebSocket` constructor rejects every scheme but `ws`/`wss`. So Rust
//! is the WebSocket client here — it dials the daemon's existing unix socket
//! (where a WS upgrade is perfectly legal) and bridges an arbitrary `/v0/...`
//! WebSocket route both ways to the webview.
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
//! A connection's unix socket stays open while *either* split half is alive, so both
//! the reader and the writer task must finish before its descriptor comes back. The
//! app is not rich in descriptors — macOS starts a GUI app at launchd's 256, and Go
//! raises its own limit while Rust does not — so a connection left half-alive burns
//! one for the life of the process.
//!
//! A connection can end three ways, and all three must converge on that teardown:
//!
//!   1. the daemon closes it (every watcher release, every workspace switch),
//!   2. the frontend closes it (`ws_close`),
//!   3. the page it belongs to goes away (a reload orphans every connection on it).
//!
//! So a [`Connection`] owns everything the connection needs to die: dropping it drops
//! the sender, which ends the writer task, and drops the cancel channel, which ends
//! the reader task. **Removing a connection from the map is therefore the one and only
//! teardown primitive** — the reader retires itself on (1), and `ws_close` and
//! [`WsBridgeManager::close_all`] do the removing for (2) and (3).
//!
//! Note what this deliberately does *not* rely on: a dead `Channel`. `Channel::send`
//! bottoms out in `webview.eval()`, which succeeds for as long as the *webview* is
//! alive — a reloaded page is the same webview. It therefore cannot detect a reload,
//! only app shutdown, and a reader waiting to be told its page is gone would wait
//! forever. Cancellation has to be explicit; case (3) is why.

use std::collections::HashMap;
use std::path::Path;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex};

use futures_util::{SinkExt, StreamExt};
use tauri::ipc::Channel;
use tauri::State;
use tokio::net::UnixStream;
use tokio::sync::{mpsc, oneshot};
use tokio_tungstenite::client_async;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;
use tokio_tungstenite::tungstenite::Message;

/// Sentinel pushed down the message Channel when the daemon closes the stream
/// (close frame, error, or EOF). The JS shim recognises it and fires `onclose`
/// so `wsManager` reconnects and the §6 cache re-seeds. A NUL prefix makes it
/// impossible to collide with a real JSON DTO frame (which always starts `{`).
pub const WS_CLOSE_SENTINEL: &str = "\u{0}crowbar-ws-close";

/// Where daemon → webview frames go: a Tauri `Channel` in the app, a plain closure
/// under test. The indirection is what lets a connection's lifetime be exercised
/// without standing up a webview.
pub trait FrameSink: Send + Sync + 'static {
    fn send(&self, frame: String);
}

impl FrameSink for Channel<String> {
    fn send(&self, frame: String) {
        // Errors here mean the webview is gone, i.e. the app is shutting down. They
        // say nothing about the connection (in particular, a page reload does NOT
        // produce one), so there is nothing useful to do but drop the frame.
        let _ = Channel::send(self, frame);
    }
}

/// Everything a live connection needs in order to die. Dropping it ends both of the
/// connection's tasks, and so releases its socket: the sender going away ends the
/// writer, and the cancel channel going away ends the reader.
struct Connection {
    generation: u64,
    tx: mpsc::UnboundedSender<Message>,
    /// Held, never used. Its `Drop` is the signal.
    _cancel: oneshot::Sender<()>,
}

type Connections = Arc<Mutex<HashMap<String, Connection>>>;

/// Maps a connection id to its live connection. One writer task per connection
/// serialises all outbound frames so command handlers never touch the socket.
#[derive(Default)]
pub struct WsBridgeManager {
    connections: Connections,
    next_generation: AtomicU64,
}

impl WsBridgeManager {
    pub fn new() -> Self {
        Self::default()
    }

    fn sender(&self, conn_id: &str) -> Option<mpsc::UnboundedSender<Message>> {
        self.connections
            .lock()
            .unwrap()
            .get(conn_id)
            .map(|c| c.tx.clone())
    }

    /// Retires every connection. Called when a page load orphans them all: the JS
    /// that owned these ids is gone and will never call `ws_close` for them, and the
    /// new page opens fresh ids of its own.
    pub fn close_all(&self) {
        self.connections.lock().unwrap().clear();
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
    open_bridge(&socket, conn_id, path, on_message, &manager)
        .await
        .map(|_reader| ())
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
) -> Result<tokio::task::JoinHandle<()>, String> {
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

    // Writer task: drain the mpsc and push frames at the socket one at a time. It
    // ends when every sender is dropped — which is what retiring the connection does.
    let (tx, mut rx) = mpsc::unbounded_channel::<Message>();
    let writer = tokio::spawn(async move {
        while let Some(msg) = rx.recv().await {
            if write.send(msg).await.is_err() {
                break;
            }
        }
        let _ = write.close().await;
    });

    let generation = manager.next_generation.fetch_add(1, Ordering::Relaxed);
    let (cancel, mut cancelled) = oneshot::channel();

    // Register before the reader can retire itself, so a connection the daemon closes
    // instantly still finds its own entry to remove. A re-used id would otherwise
    // strand the previous connection, so the displaced one is dropped — which retires
    // it in full, by the same one primitive.
    manager.connections.lock().unwrap().insert(
        conn_id.clone(),
        Connection {
            generation,
            tx,
            _cancel: cancel,
        },
    );

    // Reader task: forward each text frame verbatim to the webview channel until the
    // daemon closes the stream or this connection is retired out from under us.
    let connections = Arc::clone(&manager.connections);
    let reader = tokio::spawn(async move {
        let mut daemon_closed = false;
        loop {
            tokio::select! {
                // The connection was removed from the map: `ws_close`, a page load, or a
                // re-used id. Whoever removed it has already retired it; just let go of
                // the read half so the socket can close.
                //
                // Ending the writer half-closes the socket, and a healthy daemon answers
                // that by closing its end — which would wake this reader anyway. But a
                // daemon that has stopped reading (the wedge the watchdog exists for)
                // never will, and then the reader parks here for the life of the app,
                // holding the descriptor. Teardown must not depend on the peer.
                _ = &mut cancelled => break,
                frame = read.next() => match frame {
                    Some(Ok(Message::Text(text))) => on_message.send(text),
                    Some(Ok(_)) => continue,
                    Some(Err(_)) | None => {
                        daemon_closed = true;
                        break;
                    }
                },
            }
        }
        drop(read);

        // The daemon ended it, so nobody else will retire it: remove the entry, which
        // drops the sender and lets the writer finish. Guard on the generation — if the
        // id has since been re-used, the entry belongs to a live connection and is not
        // ours to remove. When we were cancelled instead, whoever cancelled us already
        // removed it, so there is nothing to do here.
        if daemon_closed {
            let mut conns = connections.lock().unwrap();
            if conns
                .get(&conn_id)
                .is_some_and(|c| c.generation == generation)
            {
                conns.remove(&conn_id);
            }
        }

        // Both paths wait for the writer, so this task finishing means the socket's
        // other half is really gone — i.e. the descriptor is back. That makes the
        // reader's completion the connection's single, observable teardown point.
        let _ = writer.await;

        // Only tell the shim to reconnect if the daemon is what ended this. A cancelled
        // connection was closed, superseded, or orphaned by a page load, and in each case
        // whoever did it already knows. Announcing after the writer means a reconnect can
        // never race ahead of the descriptor it is about to need.
        if daemon_closed {
            on_message.send(WS_CLOSE_SENTINEL.to_string());
        }
    });

    Ok(reader)
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
            .send(Message::Text(data))
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
    use tokio::net::UnixListener;

    /// File descriptors this process holds open (`/dev/fd` on macOS, a symlink to
    /// `/proc/self/fd` on Linux). Process-wide, and therefore only meaningful while
    /// `test_support::socket_tests()` is held — see the note there.
    fn open_fds() -> usize {
        std::fs::read_dir("/dev/fd").map(|d| d.count()).unwrap_or(0)
    }

    /// Short path: a unix socket's sun_path is capped at 104 bytes and $TMPDIR on
    /// macOS is long enough to matter.
    fn test_socket(tag: &str) -> std::path::PathBuf {
        let p = std::path::PathBuf::from(format!("/tmp/cbws-{}-{tag}.sock", std::process::id()));
        let _ = std::fs::remove_file(&p);
        p
    }

    struct ClosureSink<F>(F);

    impl<F: Fn(String) + Send + Sync + 'static> FrameSink for ClosureSink<F> {
        fn send(&self, frame: String) {
            (self.0)(frame)
        }
    }

    /// A sink that fires `tx` when the close sentinel arrives, so a test can block on
    /// the connection actually being torn down rather than on a clock.
    fn sentinel_sink(tx: oneshot::Sender<()>) -> impl FrameSink {
        let tx = Mutex::new(Some(tx));
        ClosureSink(move |frame: String| {
            if frame == WS_CLOSE_SENTINEL {
                if let Some(tx) = tx.lock().unwrap().take() {
                    let _ = tx.send(());
                }
            }
        })
    }

    fn silent_sink() -> impl FrameSink {
        ClosureSink(|_| {})
    }

    /// The daemon closing a stream on its own initiative — what it does on every
    /// watcher release and workspace switch, and what the `files/ws` churn in a
    /// pre-crash daemon log is made of.
    async fn accept_then_close(listener: &UnixListener) {
        if let Ok((stream, _)) = listener.accept().await {
            if let Ok(ws) = tokio_tungstenite::accept_async(stream).await {
                drop(ws);
            }
        }
    }

    /// A wedged daemon: it completes the upgrade and then never touches the socket again
    /// — never reads it, never answers, never closes it.
    ///
    /// Getting this peer right is the whole point of the two tests below. Retiring a
    /// connection ends the writer, which half-closes our socket (`SHUT_WR`) — and a
    /// *healthy* daemon reacts to that EOF by closing its end, which wakes our reader all
    /// by itself. So a fake daemon that reads its socket cannot tell a working teardown
    /// from a broken one: both look identical. This one never reads, so nothing but our
    /// own cancellation can free the descriptor, which is precisely the case cancellation
    /// exists for — and the case a wedged daemon actually presents.
    fn spawn_wedged_daemon(listener: UnixListener) {
        tokio::spawn(async move {
            if let Ok((stream, _)) = listener.accept().await {
                if let Ok(_ws) = tokio_tungstenite::accept_async(stream).await {
                    // Hold the connection open forever, without ever polling it.
                    std::future::pending::<()>().await;
                }
            }
        });
    }

    #[tokio::test]
    async fn daemon_initiated_close_retires_the_connection() {
        let _serialised = crate::test_support::socket_tests().await;

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
            "the daemon closed the stream, so the connection must be retired; a connection \
             left in the map strands its writer task on the socket's write half and burns \
             one of the app's descriptors for good"
        );

        let _ = std::fs::remove_file(&sock);
    }

    /// The production bug, as a regression test: 215 of these closes in one window is what
    /// a pre-crash daemon log shows, against an app whose descriptor limit was 256.
    ///
    /// This is the one test that has to count descriptors. A stranded writer still lets its
    /// reader finish, so nothing local to the connection reveals it — only the process's
    /// descriptor table does.
    #[tokio::test]
    async fn daemon_initiated_closes_do_not_leak_file_descriptors() {
        let _serialised = crate::test_support::socket_tests().await;

        const CYCLES: usize = 20;

        let sock = test_socket("fds");
        let listener = UnixListener::bind(&sock).unwrap();
        let manager = WsBridgeManager::new();
        let mut steady_state = 0;

        // Every cycle is one reconnect: the shim opens a fresh conn_id, the daemon closes it.
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

            // After the first cycle the runtime's own descriptors are all allocated, so this
            // is the level a leak-free bridge must return to every time.
            if i == 0 {
                steady_state = open_fds();
            }
        }

        let after = open_fds();
        assert!(
            after <= steady_state,
            "{CYCLES} daemon-initiated closes leaked file descriptors: {steady_state} -> {after}. \
             Once the app's limit is reached the daemon socket cannot be dialled at all — which \
             the health watchdog reads as a dead backend and kills a daemon that was never \
             unhealthy."
        );

        let _ = std::fs::remove_file(&sock);
    }

    /// The reader parks on `read.next()`. Against a daemon that has stopped reading, the
    /// half-close that retiring performs is never noticed and never answered, so nothing but
    /// explicit cancellation can end the reader — and this test hangs.
    ///
    /// The reader finishing IS the assertion: it awaits the writer, so by the time it returns
    /// both split halves are dropped and the socket is closed by ownership. Counting
    /// descriptors here would add nothing and would only make the test race every other test
    /// that opens one.
    #[tokio::test]
    async fn explicit_close_releases_the_socket_against_a_daemon_that_never_reacts() {
        let _serialised = crate::test_support::socket_tests().await;

        let sock = test_socket("close");
        spawn_wedged_daemon(UnixListener::bind(&sock).unwrap());
        let manager = WsBridgeManager::new();

        let reader = open_bridge(
            &sock,
            "c1".to_string(),
            "/v0/x".to_string(),
            silent_sink(),
            &manager,
        )
        .await
        .unwrap();

        manager.connections.lock().unwrap().remove("c1");

        reader
            .await
            .expect("retiring a connection must end its reader, whatever the daemon does");

        let _ = std::fs::remove_file(&sock);
    }

    /// A page load orphans every connection the old page owned: its JS is gone and will never
    /// call `ws_close` for ids it no longer remembers. Nothing else can notice — a `Channel`
    /// keeps working across a reload — so `close_all` is all that stands between a reload and
    /// a permanently stranded socket.
    #[tokio::test]
    async fn close_all_retires_connections_a_reloaded_page_abandoned() {
        let _serialised = crate::test_support::socket_tests().await;

        let sock = test_socket("reload");
        spawn_wedged_daemon(UnixListener::bind(&sock).unwrap());
        let manager = WsBridgeManager::new();

        let reader = open_bridge(
            &sock,
            "c1".to_string(),
            "/v0/x".to_string(),
            silent_sink(),
            &manager,
        )
        .await
        .unwrap();

        manager.close_all();

        reader.await.expect("a page load must end its readers");

        assert!(manager.connections.lock().unwrap().is_empty());

        let _ = std::fs::remove_file(&sock);
    }
}
