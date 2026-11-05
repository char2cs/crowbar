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
mod tests;
