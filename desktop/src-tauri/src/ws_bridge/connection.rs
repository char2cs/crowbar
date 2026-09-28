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
//!   1. the daemon closes it (every watcher release, every workspace switch), or the
//!      socket dies under it — a failed write, or (for a connection opened with a
//!      read-idle timeout) a half-open socket that delivers nothing at all,
//!   2. the frontend closes it (`ws_close`),
//!   3. the page it belongs to goes away (a reload orphans every connection on it).
//!
//! So a [`Connection`] owns everything the connection needs to die: dropping it drops
//! the sender, which ends the writer task, and drops the cancel channel, which ends
//! the reader task. **Removing a connection from the map is therefore the one and only
//! teardown primitive** — the reader retires itself on (1), and `ws_close` and
//! [`WsBridgeManager::close_for_window`] do the removing for (2) and (3).
//!
//! Note what this deliberately does *not* rely on: a dead `Channel`. `Channel::send`
//! bottoms out in `webview.eval()`, which succeeds for as long as the *webview* is
//! alive — a reloaded page is the same webview. It therefore cannot detect a reload,
//! only app shutdown, and a reader waiting to be told its page is gone would wait
//! forever. Cancellation has to be explicit; case (3) is why.

use std::collections::HashMap;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use futures_util::{SinkExt, StreamExt};
use tokio::io::{AsyncRead, AsyncWrite};
use tokio::sync::{mpsc, oneshot};
use tokio::time::timeout;
use tokio_tungstenite::tungstenite::Message;
use tokio_tungstenite::WebSocketStream;

use super::{Frame, FrameSink, WS_CLOSE_SENTINEL};

/// Everything a live connection needs in order to die. Dropping it ends both of the
/// connection's tasks, and so releases its socket: the sender going away ends the
/// writer, and the cancel channel going away ends the reader.
pub(super) struct Connection {
    generation: u64,
    /// The Tauri window label that opened this connection. A page load retires only
    /// the connections belonging to the window that loaded — see `close_for_window`.
    window_label: String,
    /// The paired host this connection reaches, `None` for the local daemon. Removing
    /// a host retires only its connections — see `close_for_host`.
    host_id: Option<String>,
    tx: mpsc::UnboundedSender<Message>,
    /// Held, never used. Its `Drop` is the signal.
    _cancel: oneshot::Sender<()>,
}

type Connections = Arc<Mutex<HashMap<String, Connection>>>;

/// Maps a connection id to its live connection. One writer task per connection
/// serialises all outbound frames so command handlers never touch the socket.
#[derive(Default)]
pub struct WsBridgeManager {
    pub(super) connections: Connections,
    next_generation: AtomicU64,
}

impl WsBridgeManager {
    pub fn new() -> Self {
        Self::default()
    }

    pub(super) fn sender(&self, conn_id: &str) -> Option<mpsc::UnboundedSender<Message>> {
        self.connections
            .lock()
            .unwrap()
            .get(conn_id)
            .map(|c| c.tx.clone())
    }

    /// Retires every connection belonging to `label`. Called when a page load orphans
    /// that window's connections: the JS that owned these ids is gone and will never
    /// call `ws_close` for them, and the new page opens fresh ids of its own.
    ///
    /// Scoped to one window, never app-wide: with two windows open, an app-wide clear
    /// means window B merely reloading strands every socket window A owns. Connection
    /// ids are globally unique (`crypto.randomUUID` on the JS side), so the id-keyed
    /// map needs no re-keying — only teardown is window-aware.
    pub fn close_for_window(&self, label: &str) {
        self.connections
            .lock()
            .unwrap()
            .retain(|_, c| c.window_label != label);
    }

    /// Retires every connection to paired host `host_id`. Called when the host is
    /// removed, so nothing keeps streaming from, or typing into, a host the user dropped.
    pub fn close_for_host(&self, host_id: &str) {
        self.connections
            .lock()
            .unwrap()
            .retain(|_, c| c.host_id.as_deref() != Some(host_id));
    }
}

/// Bridges an upgraded stream until it is retired, and returns its reader task. Local and
/// remote connections both live and die here, so their lifetimes cannot drift apart.
pub(super) fn attach<T, S>(
    ws: WebSocketStream<T>,
    conn_id: String,
    host_id: Option<String>,
    on_message: S,
    manager: &WsBridgeManager,
    window_label: String,
    read_idle_timeout: Option<Duration>,
) -> tokio::task::JoinHandle<()>
where
    T: AsyncRead + AsyncWrite + Unpin + Send + 'static,
    S: FrameSink,
{
    let (mut write, mut read) = ws.split();

    // Writer task: drain the mpsc and push frames at the socket one at a time. It
    // ends when every sender is dropped — which is what retiring the connection does.
    //
    // A send FAILURE is not a retirement: it is the socket's write direction going
    // dead, and a keystroke that vanished with neither error nor reconnect is exactly
    // the half-open bug. Signal it on `writer_dead` so the reader — which owns the one
    // retire + announce path — wakes and surfaces it.
    let (tx, mut rx) = mpsc::unbounded_channel::<Message>();
    let (writer_dead_tx, mut writer_dead) = oneshot::channel::<()>();
    let writer = tokio::spawn(async move {
        while let Some(msg) = rx.recv().await {
            if write.send(msg).await.is_err() {
                let _ = writer_dead_tx.send(());
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
            window_label,
            host_id,
            tx,
            _cancel: cancel,
        },
    );

    // Reader task: forward each frame verbatim to the webview channel until the
    // daemon closes the stream, the socket dies, or this connection is retired out
    // from under us.
    let connections = Arc::clone(&manager.connections);
    let reader = tokio::spawn(async move {
        let mut daemon_closed = false;
        loop {
            // A read, bounded by the idle timeout when one was asked for. `Err` means the
            // socket delivered nothing — not a frame, not a ping — for the whole window.
            let next = async {
                match read_idle_timeout {
                    Some(idle) => timeout(idle, read.next()).await,
                    None => Ok(read.next().await),
                }
            };
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
                // The writer hit a socket error (`Ok`): the transport is lost. `Err` is
                // the writer's queue closing on a normal retirement, which the cancel
                // branch owns.
                writer_result = &mut writer_dead => {
                    daemon_closed = writer_result.is_ok();
                    break;
                }
                frame = next => match frame {
                    Ok(Some(Ok(Message::Text(text)))) => {
                        on_message.send(Frame::Text(text.to_string()))
                    }
                    Ok(Some(Ok(Message::Binary(bytes)))) => {
                        on_message.send(Frame::Binary(bytes.to_vec()))
                    }
                    Ok(Some(Ok(_))) => continue,
                    Ok(Some(Err(_))) | Ok(None) | Err(_) => {
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
            on_message.send(Frame::Text(WS_CLOSE_SENTINEL.to_string()));
        }
    });

    reader
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ws_bridge::open_bridge;
    use crate::ws_bridge::test_support::{
        accept_send_binary_then_idle, accept_then_close, open_fds, sentinel_sink, silent_sink,
        spawn_wedged_daemon, spawn_wedged_daemon_multi, test_socket, ClosureSink,
    };
    use tokio::net::UnixListener;

    #[tokio::test]
    async fn daemon_initiated_close_retires_the_connection() {
        let _serialised = crate::test_support::fd_tests().await;

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
            "main".to_string(),
            None,
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
        let _serialised = crate::test_support::fd_tests().await;

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
                "main".to_string(),
                None,
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
        let _serialised = crate::test_support::fd_tests().await;

        let sock = test_socket("close");
        spawn_wedged_daemon(UnixListener::bind(&sock).unwrap());
        let manager = WsBridgeManager::new();

        let reader = open_bridge(
            &sock,
            "c1".to_string(),
            "/v0/x".to_string(),
            silent_sink(),
            &manager,
            "main".to_string(),
            None,
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
    /// keeps working across a reload — so `close_for_window` is all that stands between a
    /// reload and a permanently stranded socket.
    #[tokio::test]
    async fn a_page_load_retires_the_connections_a_reloaded_page_abandoned() {
        let _serialised = crate::test_support::fd_tests().await;

        let sock = test_socket("reload");
        spawn_wedged_daemon(UnixListener::bind(&sock).unwrap());
        let manager = WsBridgeManager::new();

        let reader = open_bridge(
            &sock,
            "c1".to_string(),
            "/v0/x".to_string(),
            silent_sink(),
            &manager,
            "main".to_string(),
            None,
        )
        .await
        .unwrap();

        manager.close_for_window("main");

        reader.await.expect("a page load must end its readers");

        assert!(manager.connections.lock().unwrap().is_empty());

        let _ = std::fs::remove_file(&sock);
    }

    /// The multi-window invariant: a page load in ONE window must not touch another
    /// window's connections. Before window scoping this was `close_all`, so window B
    /// merely opening — or reloading — silently stranded every socket window A owned.
    #[tokio::test]
    async fn a_page_load_retires_only_the_loading_windows_connections() {
        let _serialised = crate::test_support::fd_tests().await;

        let sock = test_socket("window-scope");
        spawn_wedged_daemon_multi(UnixListener::bind(&sock).unwrap());
        let manager = WsBridgeManager::new();

        let main_reader = open_bridge(
            &sock,
            "main-conn".to_string(),
            "/v0/x".to_string(),
            silent_sink(),
            &manager,
            "main".to_string(),
            None,
        )
        .await
        .unwrap();

        let w2_reader = open_bridge(
            &sock,
            "w2-conn".to_string(),
            "/v0/x".to_string(),
            silent_sink(),
            &manager,
            "w2".to_string(),
            None,
        )
        .await
        .unwrap();

        manager.close_for_window("w2");

        w2_reader
            .await
            .expect("the loading window's own reader must end");

        {
            let conns = manager.connections.lock().unwrap();
            assert!(
                conns.contains_key("main-conn"),
                "window `main`'s connection must survive window `w2`'s page load"
            );
            assert!(
                !conns.contains_key("w2-conn"),
                "window `w2`'s own connection must be retired by its page load"
            );
        }

        // Clean up the surviving connection so the test leaves no live socket behind.
        manager.close_for_window("main");
        main_reader.await.expect("teardown must end the reader");

        let _ = std::fs::remove_file(&sock);
    }

    /// The terminal's PTY output is binary: it must reach the webview as the exact
    /// bytes, not re-encoded as text.
    #[tokio::test]
    async fn binary_frames_are_forwarded_as_raw_bytes() {
        let _serialised = crate::test_support::fd_tests().await;

        let sock = test_socket("binary");
        let listener = UnixListener::bind(&sock).unwrap();
        let payload = vec![0u8, b'h', b'i', 0xF0, 0x9F, 0x9A, 0x80];
        tokio::spawn(accept_send_binary_then_idle(listener, payload.clone()));
        let manager = WsBridgeManager::new();

        let (tx, rx) = oneshot::channel();
        let tx = Mutex::new(Some(tx));
        let reader = open_bridge(
            &sock,
            "c1".to_string(),
            "/v0/x".to_string(),
            ClosureSink(move |frame: Frame| {
                if let Some(tx) = tx.lock().unwrap().take() {
                    let _ = tx.send(frame);
                }
            }),
            &manager,
            "main".to_string(),
            None,
        )
        .await
        .unwrap();

        assert_eq!(rx.await.unwrap(), Frame::Binary(payload));

        manager.connections.lock().unwrap().remove("c1");
        reader.await.unwrap();
        let _ = std::fs::remove_file(&sock);
    }

    /// A socket that goes HALF-OPEN delivers nothing — not a frame, not a ping — while
    /// the daemon still lists the session as live. With a read-idle timeout the
    /// connection is judged dead, retired, and announced like a daemon close, so the
    /// frontend re-attaches instead of typing into a corpse.
    #[tokio::test]
    async fn a_silent_half_open_socket_is_retired_and_announced() {
        let _serialised = crate::test_support::fd_tests().await;

        let sock = test_socket("halfopen");
        spawn_wedged_daemon(UnixListener::bind(&sock).unwrap());
        let manager = WsBridgeManager::new();

        let (tx, closed) = oneshot::channel();
        let reader = open_bridge(
            &sock,
            "c1".to_string(),
            "/v0/x".to_string(),
            sentinel_sink(tx),
            &manager,
            "main".to_string(),
            Some(Duration::from_millis(50)),
        )
        .await
        .unwrap();

        reader
            .await
            .expect("a silent half-open socket must be judged dead by the read-idle timeout");
        closed
            .await
            .expect("a half-open socket must be announced so the frontend re-attaches");
        assert!(
            manager.connections.lock().unwrap().is_empty(),
            "a half-open connection must be retired, not left holding its socket"
        );

        let _ = std::fs::remove_file(&sock);
    }
}
