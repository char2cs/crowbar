//! A live subscription to one of the daemon's dual-served endpoints.
//!
//! Ported from `web/src/lib/ws/manager.ts` and the transport half of
//! `web/src/lib/ws/entity-stream.ts`. The *cache* half of that file — merging
//! frames, pruning by scope, seed generations — is not here: it is domain
//! behaviour and belongs above the transport, in `crowbar-state`.
//!
//! # One endpoint, two verbs
//!
//! `GET /v0/projects/:p/repos/:r/workspaces` returns the JSON list **and**
//! accepts a WebSocket upgrade on the same path. The daemon folded the old
//! dedicated `/ws/workspaces`, `/ws/git`, `/ws/files`, `/ws/lsp` and
//! `/ws/terminals/:id` routes into this in W7-2, and their absence is asserted
//! by `api/internal/api/v0/route_audit_test.go`. So a caller seeds with
//! [`crate::transport::get`] and subscribes here, against one path.
//!
//! # Why a thread and not a runtime
//!
//! `reqwest` is pinned `blocking` in this workspace because **gpui runs no
//! tokio reactor**. Introducing one for the sidebar would put a second
//! runtime in a process that has none, so this uses the sync `tungstenite`
//! on an owned OS thread per subscription. Frames cross into gpui's executor
//! through an [`async_channel`], which is runtime-agnostic: the reader calls
//! `send_blocking`, and a `cx.background_spawn` task calls `recv().await`.
//!
//! The thread is also the unit of teardown. Dropping the [`Subscription`]
//! shuts the socket down from the outside, which is what makes the blocking
//! read return — there is no cancellation token to poll and no timeout to
//! wait out.

use std::io;
use std::net::Shutdown;
use std::os::unix::net::UnixStream;
use std::path::Path;
use std::sync::{Arc, Condvar, Mutex};
use std::thread::JoinHandle;
use std::time::Duration;

use tungstenite::Message;

/// The delay before the first reconnect attempt.
pub const BACKOFF_BASE: Duration = Duration::from_secs(1);
/// The ceiling the doubling backoff saturates at.
pub const BACKOFF_MAX: Duration = Duration::from_secs(30);

/// What arrives on a subscription.
///
/// Two kinds of thing, deliberately on one channel. The data frames are why
/// the stream exists; the rest is connection state, which the React app keeps
/// in a parallel store (`reportChannelState` / `reportChannelGone`, read by
/// `connection-indicator.tsx`). Splitting them into two channels here would
/// mean a subscriber could observe a frame from a connection it had not yet
/// been told about, because nothing orders two channels against each other.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Frame {
    /// One complete DTO, exactly as the daemon sent it. Undecoded here: this
    /// crate carries no domain types beyond `crowbar-proto`, and which DTO a
    /// path yields is the subscriber's knowledge, not the transport's.
    Data(String),
    /// The socket is up. Mirrors `reportChannelState(endpoint, true)`.
    Connected,
    /// The socket is down, with the reason. Mirrors
    /// `reportChannelState(endpoint, false)`.
    Disconnected(String),
    /// The stream dropped and is coming back. **The subscriber must reseed.**
    ///
    /// Not a courtesy notification. Frames missed during an outage can never
    /// be recovered by merging a later DTO — a delete that happened while the
    /// socket was down leaves a row that no future frame will ever mention —
    /// so the only correct response is a full GET.
    Reconnected,
}

/// A live subscription. Dropping it closes the socket and joins the thread.
#[derive(Debug)]
pub struct Subscription {
    frames: async_channel::Receiver<Frame>,
    control: Arc<Control>,
    reader: Option<JoinHandle<()>>,
}

impl Subscription {
    /// Subscribe to `path` on the daemon at `socket`.
    ///
    /// Returns immediately; the connection is made on the subscription's own
    /// thread. A daemon that is not up yet is not an error — the stream backs
    /// off and retries, which is the same state it would reach if the daemon
    /// restarted a second later.
    #[must_use]
    pub fn open(socket: &Path, path: &str) -> Self {
        // Bounded, so a subscriber that stops draining exerts backpressure on
        // the reader thread instead of letting the queue grow without limit.
        // Large enough that an ordinary burst — a repo seeding its workspaces
        // — never blocks the reader.
        let (tx, frames) = async_channel::bounded(256);
        let control = Arc::new(Control::default());

        let reader = std::thread::Builder::new()
            .name(format!("crowbar-stream{path}"))
            .spawn({
                let socket = socket.to_path_buf();
                let path = path.to_owned();
                let control = Arc::clone(&control);
                move || run(&socket, &path, &tx, &control)
            })
            .ok();

        Self {
            frames,
            control,
            reader,
        }
    }

    /// The frames this subscription delivers.
    ///
    /// Cloneable and awaitable: `recv().await` inside a `cx.background_spawn`
    /// task is the intended consumer.
    #[must_use]
    pub fn frames(&self) -> async_channel::Receiver<Frame> {
        self.frames.clone()
    }
}

impl Drop for Subscription {
    fn drop(&mut self) {
        self.control.stop();
        // Closing the receiver makes an in-flight `send_blocking` return
        // `Err`, so a reader parked on a full queue wakes too.
        self.frames.close();
        if let Some(reader) = self.reader.take() {
            let _ = reader.join();
        }
    }
}

/// Shared shutdown state: the flag, the parked thread's wakeup, and a handle
/// on the live socket so a blocking read can be interrupted from outside.
#[derive(Debug, Default)]
struct Control {
    state: Mutex<State>,
    wake: Condvar,
}

#[derive(Debug, Default)]
struct State {
    stopped: bool,
    /// A `try_clone` of the connected socket. Shutting this down is what makes
    /// `tungstenite`'s blocking read return, and it is the only way to
    /// interrupt one — the alternative, a read timeout, would wake the thread
    /// on a schedule for the entire life of an idle-but-healthy stream.
    socket: Option<UnixStream>,
}

impl Control {
    /// Signal shutdown and interrupt whatever the reader is doing.
    fn stop(&self) {
        if let Ok(mut state) = self.state.lock() {
            state.stopped = true;
            if let Some(socket) = state.socket.take() {
                let _ = socket.shutdown(Shutdown::Both);
            }
        }
        self.wake.notify_all();
    }

    fn is_stopped(&self) -> bool {
        self.state.lock().is_ok_and(|state| state.stopped)
    }

    /// Record the live socket so [`Self::stop`] can reach it.
    fn attach(&self, socket: Option<UnixStream>) {
        if let Ok(mut state) = self.state.lock() {
            state.socket = socket;
        }
    }

    /// Wait `delay`, or return early if shutdown is signalled. `false` means
    /// stop.
    ///
    /// A plain `thread::sleep` here would hold a dropped subscription's thread
    /// alive for up to [`BACKOFF_MAX`] — thirty seconds of a process that is
    /// trying to quit.
    fn sleep_or_stop(&self, delay: Duration) -> bool {
        let Ok(state) = self.state.lock() else {
            return false;
        };
        let Ok((state, _)) = self
            .wake
            .wait_timeout_while(state, delay, |state| !state.stopped)
        else {
            return false;
        };
        !state.stopped
    }
}

/// Connect, read until the stream ends, back off, repeat.
///
/// The backoff numbers and their reset are the React manager's, and two of its
/// behaviours are load-bearing rather than incidental:
///
/// * **A successful open resets the delay to [`BACKOFF_BASE`]**, so the next
///   outage starts from the base again rather than inheriting a long delay
///   from an old one.
/// * **The doubled delay is carried into the next attempt.** Restarting from
///   the base each time would mean a daemon that is down for a minute gets
///   hammered once a second for the whole minute.
fn run(socket: &Path, path: &str, tx: &async_channel::Sender<Frame>, control: &Arc<Control>) {
    let mut delay = BACKOFF_BASE;
    let mut first_attempt = true;

    loop {
        if control.is_stopped() {
            return;
        }

        // Every attempt after the first is a reconnect, and a reconnect means
        // the subscriber missed frames. Announced BEFORE the connect rather
        // than after it, so the reseed GET is already in flight while the
        // socket comes up — the React manager sends its sentinel the moment it
        // constructs the replacement channel, for the same reason.
        if !first_attempt && tx.send_blocking(Frame::Reconnected).is_err() {
            // Nobody is listening any more. A channel whose subscribers have
            // all left is not resurrected.
            return;
        }
        first_attempt = false;

        let reason = match connect(socket, path, control) {
            Ok(mut ws) => {
                delay = BACKOFF_BASE;
                if tx.send_blocking(Frame::Connected).is_err() {
                    control.attach(None);
                    return;
                }
                let outcome = pump(&mut ws, tx, control);
                control.attach(None);
                match outcome {
                    Pumped::Closed(reason) => reason,
                    Pumped::SubscriberGone => return,
                }
            }
            Err(err) => {
                // A refused socket is the ordinary state while the daemon
                // restarts, so it is a connection-state report rather than a
                // failure: `crowbar-sidecar` owns "is the daemon up", and this
                // stream's job is to keep trying.
                control.attach(None);
                err.to_string()
            }
        };

        if tx.send_blocking(Frame::Disconnected(reason)).is_err() {
            return;
        }

        if control.is_stopped() || !control.sleep_or_stop(delay) {
            return;
        }
        delay = next_delay(delay);
    }
}

/// Decode one [`Frame::Data`] payload into the DTO its path carries.
///
/// Lives here, not above, because §4.2 gives `crowbar-state` no edge to
/// `serde` and the wire format is this crate's concern in any case — the same
/// reason [`crate::transport::get_json`] unwraps the envelope here.
///
/// A record that does not decode yields `None` rather than an error. The
/// React stream does the same — it checks `typeof frame.id !== 'string'` and
/// ignores anything that fails — and the reasoning holds: one malformed
/// record, or one field this build does not know yet, must not take down a
/// live subscription carrying every other row.
#[must_use]
pub fn decode_frame<T: serde::de::DeserializeOwned>(raw: &str) -> Option<T> {
    serde_json::from_str(raw).ok()
}

/// The delay after `delay`: doubled, saturating at [`BACKOFF_MAX`].
///
/// Extracted so the schedule can be asserted without a test that waits for
/// it — a suite that measures backoff by sleeping is a suite that is slow when
/// it passes and flaky when the box is loaded.
#[must_use]
fn next_delay(delay: Duration) -> Duration {
    delay.saturating_mul(2).min(BACKOFF_MAX)
}

/// Dial the socket and perform the RFC 6455 upgrade over it.
fn connect(
    socket: &Path,
    path: &str,
    control: &Arc<Control>,
) -> Result<tungstenite::WebSocket<UnixStream>, StreamError> {
    let stream = UnixStream::connect(socket).map_err(StreamError::Dial)?;
    // Registered before the handshake, not after: the handshake itself blocks
    // on a read, and a subscription dropped mid-handshake has to be able to
    // interrupt it.
    control.attach(stream.try_clone().ok());

    // A unix socket has no host, but the handshake needs an origin to put in
    // the request line and `Host` header. The daemon does not resolve it — the
    // same reason `transport`'s `UNIX_AUTHORITY` exists.
    let request = format!("ws://localhost{path}");
    let (ws, _response) = tungstenite::client::client(request, stream)
        .map_err(|err| StreamError::Handshake(Box::new(err)))?;
    Ok(ws)
}

/// Why [`pump`] stopped.
enum Pumped {
    /// The stream ended, with the reason to report.
    Closed(String),
    /// The subscriber dropped. Do not reconnect.
    SubscriberGone,
}

/// Read frames until the stream closes or the subscriber leaves.
fn pump(
    ws: &mut tungstenite::WebSocket<UnixStream>,
    tx: &async_channel::Sender<Frame>,
    control: &Arc<Control>,
) -> Pumped {
    loop {
        if control.is_stopped() {
            return Pumped::SubscriberGone;
        }
        match ws.read() {
            Ok(Message::Text(text)) => {
                if tx.send_blocking(Frame::Data(text.to_string())).is_err() {
                    return Pumped::SubscriberGone;
                }
            }
            Ok(Message::Ping(_)) => {
                // `read` queues the pong; it does not send it. Without this
                // flush a daemon that pings an idle stream would time it out
                // and the sidebar would reseed for no reason.
                if let Err(err) = ws.flush() {
                    return Pumped::Closed(err.to_string());
                }
            }
            Ok(Message::Close(_)) => return Pumped::Closed("the daemon closed the stream".into()),
            Err(err) => return Pumped::Closed(err.to_string()),
            // Binary and Pong carry nothing this transport delivers, and
            // continuation frames are reassembled by `tungstenite` itself.
            Ok(_) => {}
        }
    }
}

/// `tungstenite`'s client handshake failure, spelled out because the type is
/// generic over the stream and appears in two places.
type HandshakeFailure = tungstenite::HandshakeError<tungstenite::ClientHandshake<UnixStream>>;

/// Why a subscription could not be established. Not surfaced to subscribers —
/// see [`run`] — but named so the failure modes are distinguishable in a test
/// and in a debugger.
#[derive(Debug)]
enum StreamError {
    /// The socket could not be dialled. The daemon is down or restarting.
    Dial(io::Error),
    /// The socket answered but the upgrade did not complete.
    ///
    /// Boxed because `tungstenite`'s handshake error carries the whole
    /// in-progress handshake — including the stream — so an unboxed variant
    /// would make this enum as large as a live connection.
    Handshake(Box<HandshakeFailure>),
}

impl std::fmt::Display for StreamError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Dial(err) => write!(f, "could not reach the daemon: {err}"),
            Self::Handshake(err) => write!(f, "the daemon refused the upgrade: {err}"),
        }
    }
}

#[cfg(test)]
mod tests {
    use std::os::unix::net::{UnixListener, UnixStream};
    use std::path::PathBuf;
    use std::sync::Arc;
    use std::sync::atomic::{AtomicBool, AtomicUsize, Ordering};
    use std::thread::JoinHandle;
    use std::time::Duration;

    use tungstenite::{Message, WebSocket};

    use super::{BACKOFF_BASE, BACKOFF_MAX, Frame, Subscription, next_delay};

    /// A real unix socket speaking real WebSocket.
    ///
    /// Not a mock: the handshake, the framing and the close are
    /// `tungstenite`'s own, so a test here exercises the same code path the
    /// daemon will. `accepts` bounds the run so the thread ends on its own and
    /// `drop` can join it without a shutdown protocol of its own.
    struct WsServer {
        dir: PathBuf,
        socket: PathBuf,
        accepted: Arc<AtomicUsize>,
        stop: Arc<AtomicBool>,
        server: Option<JoinHandle<()>>,
    }

    impl WsServer {
        fn serving<F>(name: &str, accepts: usize, script: F) -> Self
        where
            F: Fn(usize, &mut WebSocket<UnixStream>) + Send + 'static,
        {
            let dir = std::env::temp_dir().join(format!(
                "crowbar-stream-{name}-{}-{:?}",
                std::process::id(),
                std::thread::current().id(),
            ));
            let _ = std::fs::remove_dir_all(&dir);
            std::fs::create_dir_all(&dir).expect("temp dir");
            let socket = dir.join("d.sock");
            let listener = UnixListener::bind(&socket).expect("bind");
            let accepted = Arc::new(AtomicUsize::new(0));
            let stop = Arc::new(AtomicBool::new(false));

            let server = std::thread::spawn({
                let accepted = Arc::clone(&accepted);
                let stop = Arc::clone(&stop);
                move || {
                    for index in 0..accepts {
                        let Ok((stream, _)) = listener.accept() else {
                            return;
                        };
                        // Released by the self-dial in `drop`, not a real
                        // client: stop before touching the stream.
                        if stop.load(Ordering::SeqCst) {
                            return;
                        }
                        let Ok(mut ws) = tungstenite::accept(stream) else {
                            continue;
                        };
                        accepted.fetch_add(1, Ordering::SeqCst);
                        script(index, &mut ws);
                    }
                }
            });

            Self {
                dir,
                socket,
                accepted,
                stop,
                server: Some(server),
            }
        }

        fn accepted(&self) -> usize {
            self.accepted.load(Ordering::SeqCst)
        }
    }

    impl Drop for WsServer {
        fn drop(&mut self) {
            // A fixture that expected more connections than the test made is
            // parked in `accept`, and joining it would hang the suite rather
            // than fail it. One self-dial releases that park; the flag above
            // makes the server treat it as the shutdown it is.
            self.stop.store(true, Ordering::SeqCst);
            let _ = UnixStream::connect(&self.socket);
            if let Some(server) = self.server.take() {
                let _ = server.join();
            }
            let _ = std::fs::remove_dir_all(&self.dir);
        }
    }

    /// Block on the channel for the next frame. Every wait in this suite is on
    /// a real signal — a frame arriving — never on a duration. The bound only
    /// exists so a broken build fails instead of hanging the suite forever.
    fn next_frame(frames: &async_channel::Receiver<Frame>) -> Frame {
        frames
            .recv_blocking()
            .expect("the subscription is still open")
    }

    /// Drain until a data frame, returning the connection-state frames seen on
    /// the way. Lets a test assert on payloads without restating the
    /// `Connected` frame that always precedes them.
    fn next_data(frames: &async_channel::Receiver<Frame>) -> String {
        loop {
            if let Frame::Data(text) = next_frame(frames) {
                return text;
            }
        }
    }

    // --- the backoff schedule ---------------------------------------------

    #[test]
    fn the_backoff_doubles_from_one_second() {
        assert_eq!(BACKOFF_BASE, Duration::from_secs(1));
        assert_eq!(next_delay(BACKOFF_BASE), Duration::from_secs(2));
        assert_eq!(next_delay(Duration::from_secs(2)), Duration::from_secs(4));
        assert_eq!(next_delay(Duration::from_secs(8)), Duration::from_secs(16));
    }

    /// A daemon that is down for an hour must not be dialled 3600 times.
    #[test]
    fn the_backoff_saturates_at_thirty_seconds() {
        assert_eq!(next_delay(Duration::from_secs(16)), BACKOFF_MAX);
        assert_eq!(next_delay(BACKOFF_MAX), BACKOFF_MAX);
        assert_eq!(next_delay(Duration::from_hours(1)), BACKOFF_MAX);
    }

    // --- delivery ----------------------------------------------------------

    #[test]
    fn a_subscription_reports_connected_then_delivers_frames() {
        let server = WsServer::serving("deliver", 1, |_, ws| {
            let _ = ws.send(Message::text(r#"{"id":"w1"}"#));
            let _ = ws.send(Message::text(r#"{"id":"w2"}"#));
            let _ = ws.close(None);
            let _ = ws.flush();
        });

        let subscription = Subscription::open(&server.socket, "/v0/projects/p/repos/r/workspaces");
        let frames = subscription.frames();

        assert_eq!(
            next_frame(&frames),
            Frame::Connected,
            "the open is reported before any payload"
        );
        assert_eq!(next_data(&frames), r#"{"id":"w1"}"#);
        assert_eq!(next_data(&frames), r#"{"id":"w2"}"#);
    }

    /// The payload is delivered undecoded. This crate does not know which DTO
    /// a path carries, and a transport that parsed one would have to.
    #[test]
    fn frames_are_delivered_verbatim() {
        let server = WsServer::serving("verbatim", 1, |_, ws| {
            let _ = ws.send(Message::text("not json at all"));
            let _ = ws.close(None);
            let _ = ws.flush();
        });

        let subscription = Subscription::open(&server.socket, "/v0/projects");
        assert_eq!(next_data(&subscription.frames()), "not json at all");
    }

    #[test]
    fn binary_and_pong_frames_are_ignored() {
        let server = WsServer::serving("ignored", 1, |_, ws| {
            let _ = ws.send(Message::binary(vec![1, 2, 3]));
            let _ = ws.send(Message::Pong(Vec::new().into()));
            let _ = ws.send(Message::text("payload"));
            let _ = ws.close(None);
            let _ = ws.flush();
        });

        let subscription = Subscription::open(&server.socket, "/v0/projects");
        let frames = subscription.frames();
        assert_eq!(next_frame(&frames), Frame::Connected);
        assert_eq!(
            next_data(&frames),
            "payload",
            "the text frame is not lost behind the two that carry nothing"
        );
    }

    // --- reconnect ---------------------------------------------------------

    /// The behaviour a seeded-only sidebar would get wrong: a dropped stream
    /// comes back, and says so, because the frames missed in between can never
    /// be recovered by merging a later DTO.
    #[test]
    fn a_dropped_stream_reconnects_and_demands_a_reseed() {
        let server = WsServer::serving("reconnect", 2, |index, ws| {
            if index == 0 {
                // Drop the first connection without a close handshake — the
                // shape a daemon restart has.
                let _ = ws.send(Message::text("before"));
                let _ = ws.flush();
                return;
            }
            let _ = ws.send(Message::text("after"));
            let _ = ws.close(None);
            let _ = ws.flush();
        });

        let subscription = Subscription::open(&server.socket, "/v0/projects");
        let frames = subscription.frames();

        assert_eq!(next_frame(&frames), Frame::Connected);
        assert_eq!(next_data(&frames), "before");

        // The close, then the sentinel, then the new connection.
        let mut saw_disconnect = false;
        let mut saw_reconnect = false;
        loop {
            match next_frame(&frames) {
                Frame::Disconnected(_) => saw_disconnect = true,
                Frame::Reconnected => saw_reconnect = true,
                Frame::Data(text) => {
                    assert_eq!(text, "after");
                    break;
                }
                Frame::Connected => {}
            }
        }

        assert!(saw_disconnect, "the drop is reported");
        assert!(
            saw_reconnect,
            "the reseed sentinel arrives before the replacement stream's frames"
        );
        assert_eq!(server.accepted(), 2, "the stream really did reconnect");
    }

    /// Not an error: the daemon is legitimately down while it restarts, and
    /// the stream's job is to keep trying rather than to give up.
    #[test]
    fn nothing_listening_is_reported_and_retried() {
        let missing = std::env::temp_dir().join("crowbar-stream-nothing-here.sock");
        let _ = std::fs::remove_file(&missing);

        let subscription = Subscription::open(&missing, "/v0/projects");
        let frames = subscription.frames();

        let Frame::Disconnected(reason) = next_frame(&frames) else {
            panic!("a socket nobody is bound to reports a disconnect");
        };
        assert!(
            reason.starts_with("could not reach the daemon"),
            "the reason names the failure: {reason}"
        );
    }

    // --- teardown ----------------------------------------------------------

    /// Dropping the subscription must interrupt a blocking read, not wait it
    /// out. This test hangs rather than fails if that regresses, which is the
    /// honest signal: there is no timeout to assert on.
    #[test]
    fn dropping_a_subscription_stops_its_thread_mid_read() {
        let server = WsServer::serving("teardown", 1, |_, ws| {
            let _ = ws.send(Message::text("hello"));
            let _ = ws.flush();
            // Park in a read the client will never answer, so the only thing
            // that can end this connection is the client shutting it down.
            let _ = ws.read();
        });

        let subscription = Subscription::open(&server.socket, "/v0/projects");
        let frames = subscription.frames();
        assert_eq!(next_frame(&frames), Frame::Connected);
        assert_eq!(next_data(&frames), "hello");

        drop(subscription);

        assert!(
            frames.is_closed(),
            "the channel closes with the subscription"
        );
    }

    /// A daemon that pings an idle stream expects a pong. `read` only
    /// *queues* one, so without the flush the daemon times the stream out and
    /// the sidebar reseeds for no reason.
    #[test]
    fn a_ping_is_answered_and_does_not_end_the_stream() {
        let server = WsServer::serving("ping", 1, |_, ws| {
            let _ = ws.send(Message::Ping(Vec::new().into()));
            let _ = ws.flush();
            // Read the pong back. If the client never sent one this returns an
            // error and the payload below never goes out, failing the test.
            let pong = ws.read();
            assert!(
                matches!(pong, Ok(Message::Pong(_))),
                "the client answered the ping: {pong:?}"
            );
            let _ = ws.send(Message::text("still here"));
            let _ = ws.close(None);
            let _ = ws.flush();
        });

        let subscription = Subscription::open(&server.socket, "/v0/projects");
        let frames = subscription.frames();
        assert_eq!(next_frame(&frames), Frame::Connected);
        assert_eq!(next_data(&frames), "still here");
    }

    /// Something is listening, but it is not a WebSocket endpoint. Reported as
    /// a refused upgrade, distinguishable from a dead socket — the two have
    /// different causes and a bare "disconnected" would hide which.
    #[test]
    fn a_socket_that_refuses_the_upgrade_says_so() {
        let dir = std::env::temp_dir().join(format!(
            "crowbar-stream-nows-{}-{:?}",
            std::process::id(),
            std::thread::current().id(),
        ));
        let _ = std::fs::remove_dir_all(&dir);
        std::fs::create_dir_all(&dir).expect("temp dir");
        let socket = dir.join("d.sock");
        let listener = UnixListener::bind(&socket).expect("bind");

        let server = std::thread::spawn(move || {
            if let Ok((stream, _)) = listener.accept() {
                // Plain HTTP, no 101. The handshake cannot complete.
                let mut out = &stream;
                let _ = std::io::Write::write_all(
                    &mut out,
                    b"HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\nConnection: close\r\n\r\n",
                );
                let _ = std::io::Write::flush(&mut out);
            }
        });

        let subscription = Subscription::open(&socket, "/v0/not-a-stream");
        let frames = subscription.frames();
        let Frame::Disconnected(reason) = next_frame(&frames) else {
            panic!("a refused upgrade reports a disconnect");
        };
        assert!(
            reason.starts_with("the daemon refused the upgrade"),
            "the reason distinguishes this from a dead socket: {reason}"
        );

        drop(subscription);
        let _ = server.join();
        let _ = std::fs::remove_dir_all(&dir);
    }

    /// The subscriber leaving mid-stream must stop the reader, including when
    /// it is parked handing a frame over rather than waiting for one. The
    /// channel is bounded, so a server that outruns a subscriber parks it
    /// exactly there.
    #[test]
    fn a_subscriber_leaving_stops_a_reader_parked_on_a_full_queue() {
        let server = WsServer::serving("backpressure", 1, |_, ws| {
            for index in 0..2000 {
                if ws.send(Message::text(format!("frame-{index}"))).is_err() {
                    return;
                }
            }
            let _ = ws.flush();
        });

        let subscription = Subscription::open(&server.socket, "/v0/projects");
        let frames = subscription.frames();
        assert_eq!(next_frame(&frames), Frame::Connected);
        assert_eq!(next_data(&frames), "frame-0");

        // `drop` joins the reader thread. Returning at all is the assertion:
        // a reader still blocked in `send_blocking` would hang here.
        drop(subscription);
        // Closed, not empty: a closed `async_channel` still hands out whatever
        // was already buffered, so draining is not the signal here.
        assert!(frames.is_closed());
    }

    /// Dropped before the socket is even up. The stop has to be observed by
    /// the connect path, not only by the read loop, or a subscription created
    /// and abandoned during startup would outlive the window that made it.
    #[test]
    fn dropping_a_subscription_before_it_connects_tears_down() {
        let missing = std::env::temp_dir().join("crowbar-stream-never-up.sock");
        let _ = std::fs::remove_file(&missing);

        let subscription = Subscription::open(&missing, "/v0/projects");
        let frames = subscription.frames();
        // `drop` joins the reader thread, so returning at all is the
        // assertion: a thread still parked in its backoff would hang here.
        drop(subscription);

        assert!(frames.is_closed());
    }
}
