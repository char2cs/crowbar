//! Stand-in daemons and frame sinks for the bridge's unix-socket tests.

use std::sync::Mutex;

use futures_util::SinkExt;
use tokio::net::UnixListener;
use tokio::sync::oneshot;
use tokio_tungstenite::tungstenite::Message;

use super::{Frame, FrameSink, WS_CLOSE_SENTINEL};

/// File descriptors this process holds open (`/dev/fd` on macOS, a symlink to
/// `/proc/self/fd` on Linux). Process-wide, and therefore only meaningful while
/// `test_support::socket_tests()` is held — see the note there.
pub fn open_fds() -> usize {
    std::fs::read_dir("/dev/fd").map(|d| d.count()).unwrap_or(0)
}

/// Short path: a unix socket's sun_path is capped at 104 bytes and $TMPDIR on
/// macOS is long enough to matter.
pub fn test_socket(tag: &str) -> std::path::PathBuf {
    let p = std::path::PathBuf::from(format!("/tmp/cbws-{}-{tag}.sock", std::process::id()));
    let _ = std::fs::remove_file(&p);
    p
}

pub struct ClosureSink<F>(pub F);

impl<F: Fn(Frame) + Send + Sync + 'static> FrameSink for ClosureSink<F> {
    fn send(&self, frame: Frame) {
        (self.0)(frame)
    }
}

/// A sink that fires `tx` when the close sentinel arrives, so a test can block on
/// the connection actually being torn down rather than on a clock.
pub fn sentinel_sink(tx: oneshot::Sender<()>) -> impl FrameSink {
    let tx = Mutex::new(Some(tx));
    ClosureSink(move |frame: Frame| {
        if frame == Frame::Text(WS_CLOSE_SENTINEL.to_string()) {
            if let Some(tx) = tx.lock().unwrap().take() {
                let _ = tx.send(());
            }
        }
    })
}

pub fn silent_sink() -> impl FrameSink {
    ClosureSink(|_| {})
}

/// The daemon closing a stream on its own initiative — what it does on every
/// watcher release and workspace switch, and what the `files/ws` churn in a
/// pre-crash daemon log is made of.
pub async fn accept_then_close(listener: &UnixListener) {
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
pub fn spawn_wedged_daemon(listener: UnixListener) {
    tokio::spawn(async move {
        if let Ok((stream, _)) = listener.accept().await {
            if let Ok(_ws) = tokio_tungstenite::accept_async(stream).await {
                // Hold the connection open forever, without ever polling it.
                std::future::pending::<()>().await;
            }
        }
    });
}

/// Same wedged peer as [`spawn_wedged_daemon`], but accepting connections forever
/// rather than one. A window-scoping test needs two live connections on one
/// listener; with the single-accept version the second `open_bridge` would block on
/// a peer that never accepts.
pub fn spawn_wedged_daemon_multi(listener: UnixListener) {
    tokio::spawn(async move {
        while let Ok((stream, _)) = listener.accept().await {
            tokio::spawn(async move {
                if let Ok(_ws) = tokio_tungstenite::accept_async(stream).await {
                    std::future::pending::<()>().await;
                }
            });
        }
    });
}

/// A daemon that upgrades, sends one binary frame, then holds the stream open.
pub async fn accept_send_binary_then_idle(listener: UnixListener, payload: Vec<u8>) {
    if let Ok((stream, _)) = listener.accept().await {
        if let Ok(mut ws) = tokio_tungstenite::accept_async(stream).await {
            let _ = ws.send(Message::Binary(payload.into())).await;
            std::future::pending::<()>().await;
        }
    }
}
