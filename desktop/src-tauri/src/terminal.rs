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
use std::path::Path;
use std::sync::{Arc, Mutex};
use std::time::Duration;

use futures_util::{SinkExt, StreamExt};
use tauri::ipc::Channel;
use tauri::{Emitter, State};
use tokio::net::UnixStream;
use tokio::sync::{mpsc, oneshot};
use tokio::time::timeout;
use tokio_tungstenite::client_async;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;

use crate::ws_bridge::FrameSink;
use tokio_tungstenite::tungstenite::Message;

/// How long the reader will tolerate a socket that delivers NOTHING — not a PTY byte,
/// not a control frame — before judging it dead.
///
/// A healthy terminal socket is never silent for long: the daemon pings every 45s
/// (`wsPingPeriod`, see api/internal/api/v0/endpoints/terminal/handlers/ws.go), so an
/// attached client sees at least a control frame within that window even when the PTY
/// is idle. If the socket goes HALF-OPEN — writes still "succeed" into a dead kernel
/// buffer while nothing is delivered in either direction — `read.next()` would park
/// here forever against a session the daemon still lists as live, and no drop would
/// ever fire: the "dead scroll" bug. Two ping periods plus margin, so ordinary load or
/// GC pauses never trip it, but a genuinely wedged socket is caught and surfaced.
const READ_IDLE_TIMEOUT: Duration = Duration::from_secs(90);

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
    sessions: Sessions,
    next_generation: std::sync::atomic::AtomicU64,
}

type Sessions = Arc<Mutex<HashMap<String, Streaming>>>;

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

    /// Retires every streaming leg. Called when a page load orphans them all: the JS
    /// that owned these sessions is gone and will never close them, and a reloaded page
    /// re-attaches the ones it still wants. The daemon keeps each PTY alive across this
    /// — only the streaming leg is torn down, never the session.
    pub fn close_all(&self) {
        self.sessions.lock().unwrap().clear();
    }
}

/// Removes `session_id`, but only while `generation` is still the current one.
///
/// Rust outlives the webview, so a reloaded page can re-attach the same session id — under
/// a new generation — while the pre-reload reader is still winding down. If that reader's
/// socket errored just before the re-attach, it arrives here holding a stale generation and
/// must not evict the transport that replaced it: doing so is the "terminal silent after
/// reload" bug. The window is a genuine race between two tasks, which is why the check
/// lives under the same lock as the removal.
fn retire_if_current(sessions: &Sessions, session_id: &str, generation: u64) -> bool {
    let mut sessions = sessions.lock().unwrap();
    match sessions.get(session_id) {
        Some(s) if s.generation == generation => {
            sessions.remove(session_id);
            true
        }
        _ => false,
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
    open_terminal_bridge(
        &socket,
        session_id,
        ws_path,
        on_data,
        &manager,
        READ_IDLE_TIMEOUT,
        move |dropped| {
            let _ = app.emit("terminal:transport-dropped", dropped);
        },
    )
    .await
    .map(|_reader| ())
}

/// The transport half of [`terminal_open`], free of Tauri's `AppHandle`/`State` so it
/// can be driven directly against a real unix-socket server. `on_drop` announces an
/// *unexpected* daemon disconnect; the app wires it to `terminal:transport-dropped`.
///
/// Returns the reader task's handle. The app ignores it — the session outlives the call
/// — but it is the only honest signal that a retired session has finished letting go of
/// its socket, so tests block on it rather than on a peer that may never react.
pub async fn open_terminal_bridge<S, D>(
    socket: &Path,
    session_id: String,
    ws_path: String,
    on_data: S,
    manager: &TerminalManager,
    read_idle_timeout: Duration,
    on_drop: D,
) -> Result<tokio::task::JoinHandle<()>, String>
where
    S: FrameSink,
    D: FnOnce(String) + Send + 'static,
{
    let stream = UnixStream::connect(socket).await.map_err(|e| {
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
    //
    // A `send` FAILURE, though, is not a retirement: it is the write direction of the
    // socket going dead (the half-open case the old code swallowed with a bare `break`,
    // so a user's keystroke or wheel vanished with neither error nor reconnect). Signal
    // it on `writer_dead` so the reader — which owns the single retire + drop-announce
    // path — wakes and surfaces it, instead of parking on `read.next()` forever.
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

    // Register FIRST so the reader task can check its own generation on exit.
    let (cancel, mut cancelled) = oneshot::channel();
    let generation = manager.register(session_id.clone(), tx, cancel);

    // Reader task: forward each text frame WHOLE to the webview channel — the TS bridge
    // parses `{data, snapshot}` for both transports in one place. It is also the SINGLE
    // owner of the retire + drop-announce path: a daemon close, a writer-side socket
    // failure, and a silent half-open socket all funnel through here so the drop fires
    // exactly once and the generation guard is honoured in one place.
    let sessions = Arc::clone(&manager.sessions);
    let reader = tokio::spawn(async move {
        let mut transport_lost = false;
        loop {
            tokio::select! {
                // The session was retired: closed, re-opened under a new generation, or
                // orphaned by a page load. Whoever removed it has already retired it —
                // just let go of the read half so the socket can close. The daemon keeps
                // a live PTY's stream open and idle, so without this the reader would
                // park here for the life of the app, holding the descriptor.
                _ = &mut cancelled => break,
                // The writer hit a socket error. `Ok(())` is a real send failure — the
                // write direction is dead, so the transport is lost and must be surfaced.
                // `Err(_)` means the writer's queue closed on a normal retirement, which
                // the cancel branch above already owns, so we just stop reading.
                writer_result = &mut writer_dead => {
                    if writer_result.is_ok() {
                        transport_lost = true;
                    }
                    break;
                }
                // A read wrapped in an idle timeout. On a healthy socket the daemon's 45s
                // pings alone keep this arm firing well inside the window; if NOTHING
                // arrives for `read_idle_timeout`, the socket is half-open (delivers
                // neither output nor pings) and must be retired rather than parked on.
                frame = timeout(read_idle_timeout, read.next()) => match frame {
                    // An error from the sink means the webview is gone (app shutdown). It
                    // does NOT mean the page reloaded — a reloaded page is the same
                    // webview — so it is not a teardown signal.
                    Ok(Some(Ok(Message::Text(text)))) => on_data.send(text.to_string()),
                    Ok(Some(Ok(_))) => continue,
                    Ok(Some(Err(_))) | Ok(None) => {
                        transport_lost = true;
                        break;
                    }
                    // Idle timeout elapsed: the socket delivered nothing for the full
                    // window — a half-open corpse the daemon still lists as live.
                    Err(_elapsed) => {
                        transport_lost = true;
                        break;
                    }
                },
            }
        }
        drop(read);

        // The daemon ended it, the writer failed, or the socket went silent — in each
        // case nobody else will retire it, so we do (guarded by generation so a stale
        // post-reload reader can't evict the session that replaced it). When we were
        // cancelled instead, `transport_lost` is false and whoever cancelled us already
        // removed the entry.
        let still_current = transport_lost && retire_if_current(&sessions, &session_id, generation);

        // Both paths wait for the writer, so this task finishing means the socket's other
        // half is really gone — i.e. the descriptor is back.
        let _ = writer.await;

        // Only an unexpected transport loss is worth announcing. A retired session was
        // closed, superseded, or orphaned by a page load — in each case whoever retired it
        // already knows, and a reloaded page must not be told a session it is about to
        // re-attach has "dropped". The JS guard (`tauriTerminals.has`) is a second net.
        if still_current {
            on_drop(session_id);
        }
    });

    Ok(reader)
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

#[cfg(test)]
mod tests {
    use super::*;
    use tokio::net::UnixListener;

    /// A read-idle timeout long enough that cancellation or a daemon close always wins
    /// first, so tests that drive retirement/close never wait on the half-open backstop.
    const TEST_IDLE: Duration = Duration::from_secs(30);

    /// Short path: a unix socket's sun_path is capped at 104 bytes.
    fn test_socket(tag: &str) -> std::path::PathBuf {
        let p = std::path::PathBuf::from(format!("/tmp/cbterm-{}-{tag}.sock", std::process::id()));
        let _ = std::fs::remove_file(&p);
        p
    }

    struct ClosureSink<F>(F);

    impl<F: Fn(String) + Send + Sync + 'static> FrameSink for ClosureSink<F> {
        fn send(&self, frame: String) {
            (self.0)(frame)
        }
    }

    fn silent_sink() -> impl FrameSink {
        ClosureSink(|_| {})
    }

    /// The daemon holding a live PTY: it upgrades, then keeps the stream open and idle
    /// — it does not read it, does not send, and does not close. This is what the real
    /// daemon does for an attached terminal that nobody is typing into, and it is the
    /// only peer that can tell a working teardown from a broken one: retiring a session
    /// half-closes our socket, and a peer that *reads* would see that EOF and close its
    /// end, waking our reader all by itself. This one never does, so only cancellation
    /// can free the descriptor.
    fn spawn_idle_pty_daemon(listener: UnixListener) {
        tokio::spawn(async move {
            if let Ok((stream, _)) = listener.accept().await {
                if let Ok(_ws) = tokio_tungstenite::accept_async(stream).await {
                    std::future::pending::<()>().await;
                }
            }
        });
    }

    /// A daemon that upgrades and immediately drops the stream — an unexpected
    /// disconnect, the one case the frontend must be told about.
    async fn accept_then_close(listener: &UnixListener) {
        if let Ok((stream, _)) = listener.accept().await {
            if let Ok(ws) = tokio_tungstenite::accept_async(stream).await {
                drop(ws);
            }
        }
    }

    /// `terminal_close` must hand the descriptor back even though the daemon keeps the
    /// PTY — and its stream — alive, which is the normal case, not an edge case.
    #[tokio::test]
    async fn closing_a_session_releases_the_socket_while_the_daemon_keeps_the_pty() {
        let _serialised = crate::test_support::fd_tests().await;

        let sock = test_socket("close");
        spawn_idle_pty_daemon(UnixListener::bind(&sock).unwrap());
        let manager = TerminalManager::new();

        let reader = open_terminal_bridge(
            &sock,
            "s1".to_string(),
            "/v0/x/ws".to_string(),
            silent_sink(),
            &manager,
            TEST_IDLE,
            |_| panic!("a session the frontend closed is not an unexpected drop"),
        )
        .await
        .unwrap();

        manager.sessions.lock().unwrap().remove("s1");

        // The reader finishing IS the assertion: it awaits the writer, so by the time it
        // returns both split halves are dropped and the socket is closed by ownership.
        // Without the cancellation path this hangs, because this daemon never reacts.
        reader
            .await
            .expect("closing a session must end its reader, whatever the daemon does");

        let _ = std::fs::remove_file(&sock);
    }

    /// A page load orphans every terminal the outgoing page had attached. The daemon keeps
    /// the PTYs, so the reloaded page re-attaches — but the old streaming legs are nobody's
    /// and must not keep their sockets.
    #[tokio::test]
    async fn close_all_releases_terminals_a_reloaded_page_abandoned() {
        let _serialised = crate::test_support::fd_tests().await;

        let sock = test_socket("reload");
        spawn_idle_pty_daemon(UnixListener::bind(&sock).unwrap());
        let manager = TerminalManager::new();

        let reader = open_terminal_bridge(
            &sock,
            "s1".to_string(),
            "/v0/x/ws".to_string(),
            silent_sink(),
            &manager,
            TEST_IDLE,
            |_| panic!("a page load must not be reported as an unexpected drop"),
        )
        .await
        .unwrap();

        manager.close_all();

        reader.await.expect("a page load must end its readers");

        assert!(manager.sessions.lock().unwrap().is_empty());

        let _ = std::fs::remove_file(&sock);
    }

    /// The one case the frontend genuinely needs to hear about: the daemon dropped the
    /// stream under a session that is still current.
    #[tokio::test]
    async fn a_daemon_disconnect_retires_the_session_and_is_announced() {
        let _serialised = crate::test_support::fd_tests().await;

        let sock = test_socket("drop");
        let listener = UnixListener::bind(&sock).unwrap();
        let manager = TerminalManager::new();

        let (dropped_tx, dropped) = oneshot::channel();
        let opened = open_terminal_bridge(
            &sock,
            "s1".to_string(),
            "/v0/x/ws".to_string(),
            silent_sink(),
            &manager,
            TEST_IDLE,
            move |id| {
                let _ = dropped_tx.send(id);
            },
        );
        let (reader, _) = tokio::join!(opened, accept_then_close(&listener));
        reader.unwrap().await.unwrap();

        assert_eq!(
            dropped.await.unwrap(),
            "s1",
            "an unexpected daemon disconnect must reach the frontend so it can reconnect"
        );
        assert!(
            manager.sessions.lock().unwrap().is_empty(),
            "a dropped session must be retired, not left holding its socket"
        );

        let _ = std::fs::remove_file(&sock);
    }

    /// The "dead scroll" bug: a socket that upgrades and then goes utterly silent — no
    /// PTY bytes, no daemon pings, no close — is HALF-OPEN. `read.next()` would park on it
    /// forever while the daemon still lists the session live, and the old code surfaced no
    /// drop, so the webview believed it was connected and never re-attached. The read-idle
    /// timeout must catch it, retire the session, and announce the drop so the frontend's
    /// transport-drop → re-attach chain fires.
    #[tokio::test]
    async fn a_silent_half_open_socket_is_retired_and_announced() {
        let _serialised = crate::test_support::fd_tests().await;

        let sock = test_socket("halfopen");
        // Upgrades, then holds the socket open and idle forever: never sends, never pings,
        // never closes — exactly the half-open corpse the timeout exists to detect.
        spawn_idle_pty_daemon(UnixListener::bind(&sock).unwrap());
        let manager = TerminalManager::new();

        let (dropped_tx, dropped) = oneshot::channel();
        let reader = open_terminal_bridge(
            &sock,
            "s1".to_string(),
            "/v0/x/ws".to_string(),
            silent_sink(),
            &manager,
            Duration::from_millis(50),
            move |id| {
                let _ = dropped_tx.send(id);
            },
        )
        .await
        .unwrap();

        // No cancellation, no close: the ONLY thing that can end this reader is the
        // idle timeout judging the socket dead. If it hangs, the backstop is broken.
        reader
            .await
            .expect("a silent half-open socket must be judged dead by the read-idle timeout");

        assert_eq!(
            dropped.await.unwrap(),
            "s1",
            "a half-open socket must reach the frontend so it can re-attach"
        );
        assert!(
            manager.sessions.lock().unwrap().is_empty(),
            "a half-open session must be retired, not left holding its socket"
        );

        let _ = std::fs::remove_file(&sock);
    }

    /// Rust outlives the webview. A reloaded page re-attaches the same session id under a
    /// NEW generation while the pre-reload reader is still winding down; if that reader's
    /// socket errored just before the re-attach, it arrives at retirement holding a stale
    /// generation. It must not evict the transport that replaced it — that is the
    /// "terminal silent after reload" bug.
    ///
    /// The window is a race between two tasks and cannot be forced through the transport,
    /// so the guard is exercised where it actually lives: under the map's lock.
    #[test]
    fn a_stale_generation_never_retires_the_session_that_replaced_it() {
        let manager = TerminalManager::new();

        let (tx, _rx) = mpsc::unbounded_channel();
        let (cancel, _cancelled) = oneshot::channel();
        let pre_reload = manager.register("s1".to_string(), tx, cancel);

        let (tx, _rx) = mpsc::unbounded_channel();
        let (cancel, _cancelled) = oneshot::channel();
        let re_attached = manager.register("s1".to_string(), tx, cancel);

        assert_ne!(
            pre_reload, re_attached,
            "each attach gets its own generation"
        );

        assert!(
            !retire_if_current(&manager.sessions, "s1", pre_reload),
            "the stale pre-reload reader must not retire the session that replaced it"
        );
        assert!(
            manager.sessions.lock().unwrap().contains_key("s1"),
            "the reloaded page's live transport was evicted by a dying stale reader"
        );

        assert!(
            retire_if_current(&manager.sessions, "s1", re_attached),
            "the current generation must still be able to retire itself"
        );
        assert!(manager.sessions.lock().unwrap().is_empty());
    }
}
