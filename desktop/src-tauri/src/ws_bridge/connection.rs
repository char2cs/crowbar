//! A connection's socket stays open while either split half lives, and descriptors are
//! scarce here (macOS starts a GUI app at 256), so removing a [`Connection`] from the map
//! is the one teardown: a daemon close, `ws_close` and a page load all converge on it.

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

    /// Retires `label`'s connections when its page loads: the old JS will never `ws_close`
    /// them. Scoped to one window, or one window reloading would strand another's sockets.
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

    // The writer ends when every sender drops (retirement). A send failure is the write
    // direction dying, not a retirement: signal `writer_dead` so the reader, which owns
    // the one retire + announce path, surfaces it instead of losing keystrokes silently.
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

    // Registered before the reader can retire itself, so an instant daemon close finds its
    // entry. A re-used id drops the displaced connection, retiring it by the same primitive.
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
                // Removed from the map (`ws_close`, page load, re-used id): drop the read
                // half. A daemon that stopped reading never answers the writer's half-close,
                // so without this the reader parks on the descriptor forever.
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

        // The daemon ended it, so nobody else will retire it. The generation guard leaves a
        // re-used id's live entry alone; a cancelled reader's entry is already gone.
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

        // Announce only a daemon-side end (a cancelled one's closer already knows), and only
        // after the writer finished, so a reconnect never races the descriptor it needs.
        if daemon_closed {
            on_message.send(Frame::Text(WS_CLOSE_SENTINEL.to_string()));
        }
    });

    reader
}

#[cfg(test)]
mod tests;
