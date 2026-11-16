//! One terminal session from creation to the end of its stream.

use std::pin::pin;
use std::sync::Arc;
use std::time::Duration;

use async_channel::Receiver;
use futures_util::StreamExt;
use futures_util::future::ready;
use protocol::{DaemonTransport, HostId, PaneId, PaneOrigin, RelayScope};
use terminal::{ClientMessage, DaemonFrame};
use tokio::net::TcpStream;
use tokio::runtime::Handle;
use tokio::time::{sleep, timeout};
use tokio_tungstenite::client_async;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;
use tokio_tungstenite::tungstenite::http::HeaderValue;

use crate::binding::Binding;
use crate::ends::Ends;
use crate::error::{Error, Result};
use crate::reconnect::Reconnect;
use crate::rest::Rest;
use crate::session_base::SessionBase;
use crate::stream::pump;
use crate::stream_end::StreamEnd;

const RELAY_SUBPROTOCOL: &str = "crowbar.relay.v1";
const TOKEN_PREFIX: &str = "crowbar.token.";
const FAILURE_COLOR: &str = "\x1b[31m";
const WARNING_COLOR: &str = "\x1b[33m";
const CONNECT_TIMEOUT: Duration = Duration::from_secs(10);
const RESET: &str = "\x1b[0m";

/// Starts terminal sessions on one daemon host.
#[derive(Clone)]
pub struct TerminalSessions {
    rest: Rest,
    runtime: Handle,
    origin: String,
}

impl std::fmt::Debug for TerminalSessions {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("TerminalSessions")
            .field("host", &self.rest.host)
            .finish_non_exhaustive()
    }
}

impl TerminalSessions {
    /// Sessions on `host` through `transport`, run on `runtime`, speaking as `origin`.
    #[must_use]
    pub fn new(
        transport: Arc<dyn DaemonTransport>,
        host: HostId,
        runtime: Handle,
        origin: &str,
    ) -> Self {
        Self {
            rest: Rest { transport, host },
            runtime,
            origin: origin.to_owned(),
        }
    }

    /// Creates a session in `base` for `pane` and connects it to `ends`. A failure is written
    /// into the terminal itself; a dropped connection reattaches to the same PTY; dropping the view ends the session and kills it.
    pub fn start(
        &self,
        base: SessionBase,
        pane: PaneId,
        ends: Ends,
    ) {
        self.spawn((base, Binding::Create), pane, ends);
    }

    /// Connects `ends` to the existing `session` of `base` without creating or killing anything,
    /// for the PTY of an agent runner.
    pub fn watch(
        &self,
        (base, session): (SessionBase, String),
        pane: PaneId,
        ends: Ends,
    ) {
        self.spawn((base, Binding::Existing(session)), pane, ends);
    }

    fn spawn(
        &self,
        (base, binding): (SessionBase, Binding),
        pane: PaneId,
        ends: Ends,
    ) {
        let session = self.clone();
        self.runtime.spawn(async move {
            session.run((&base, &binding), pane, ends).await;
        });
    }

    async fn run(
        &self,
        (base, binding): (&SessionBase, &Binding),
        pane: PaneId,
        ends: Ends,
    ) {
        let scope = RelayScope::new(self.rest.host.clone(), pane, PaneOrigin::Core);
        if let Err(error) = self.attach((base, binding), &scope, &ends).await {
            tracing::warn!(%error, "terminal session failed");
            notice(&ends, &format!("{FAILURE_COLOR}{error}{RESET}")).await;
        }
        self.rest.transport.revoke_pane(&scope);
    }

    async fn attach(
        &self,
        (base, binding): (&SessionBase, &Binding),
        scope: &RelayScope,
        ends: &Ends,
    ) -> Result<()> {
        let created = timeout(CONNECT_TIMEOUT, binding.session(&self.rest, base));
        let session = created
            .await
            .map_err(|_| timed_out("creating the session"))??;
        self.keep_attached((base, &session), scope, ends).await;
        binding.release(&self.rest, base, &session).await;
        Ok(())
    }

    async fn keep_attached(
        &self,
        target: (&SessionBase, &str),
        scope: &RelayScope,
        ends: &Ends,
    ) {
        let mut reconnect = Reconnect::default();
        while !self.round(target, scope, ends, &mut reconnect).await {}
    }

    async fn round(
        &self,
        (base, session): (&SessionBase, &str),
        scope: &RelayScope,
        ends: &Ends,
        reconnect: &mut Reconnect,
    ) -> bool {
        let outcome = self.stream(base, session, scope, ends.clone()).await;
        let reason = match outcome {
            Ok(StreamEnd::ViewGone | StreamEnd::Exited) => return true,
            Ok(StreamEnd::Lost) => {
                reconnect.connected();
                "the connection dropped".to_owned()
            },
            Err(error) => error.to_string(),
        };
        tracing::warn!(%reason, "terminal stream lost");
        !recover(ends, reconnect, &reason).await
    }

    async fn stream(
        &self,
        base: &SessionBase,
        session: &str,
        scope: &RelayScope,
        ends: Ends,
    ) -> Result<StreamEnd> {
        let endpoint = self
            .rest
            .transport
            .relay_endpoint(scope)
            .map_err(stream_error)?;
        let address = endpoint.url().trim_start_matches("ws://").to_owned();
        let tcp = within(TcpStream::connect(&address)).await?;
        let url = format!("{}{}", endpoint.url(), base.stream_path(session));
        let mut request = url.into_client_request().map_err(stream_error)?;
        let protocols = format!("{RELAY_SUBPROTOCOL}, {TOKEN_PREFIX}{}", endpoint.token());
        let headers = request.headers_mut();
        headers.insert("sec-websocket-protocol", header(&protocols)?);
        headers.insert("origin", header(&self.origin)?);
        let (socket, _) = within(client_async(request, tcp)).await?;
        Ok(pump(socket, ends).await)
    }
}

async fn within<T, E>(attempt: impl Future<Output = std::result::Result<T, E>>) -> Result<T>
where
    E: std::fmt::Display,
{
    match timeout(CONNECT_TIMEOUT, attempt).await {
        Ok(done) => done.map_err(stream_error),
        Err(_) => Err(timed_out("connecting")),
    }
}

fn timed_out(stage: &str) -> Error {
    Error::Stream(format!("timed out {stage}"))
}

async fn recover(
    ends: &Ends,
    reconnect: &mut Reconnect,
    reason: &str,
) -> bool {
    if ends.messages.is_closed() {
        return false;
    }
    let Some(delay) = reconnect.next_delay() else {
        notice(
            ends,
            &format!(
                "{WARNING_COLOR}terminal disconnected ({reason}); press a key to reconnect{RESET}"
            ),
        )
        .await;
        let pressed = next_input(&ends.messages).await;
        reconnect.connected();
        return pressed;
    };
    notice(
        ends,
        &format!("{WARNING_COLOR}terminal disconnected ({reason}); reconnecting{RESET}"),
    )
    .await;
    sleep(delay).await;
    true
}

async fn next_input(messages: &Receiver<ClientMessage>) -> bool {
    let inputs = messages
        .clone()
        .filter(|message| ready(matches!(message, ClientMessage::Input(_))));
    pin!(inputs).next().await.is_some()
}

async fn notice(
    ends: &Ends,
    text: &str,
) {
    let frame = DaemonFrame::Output(format!("\r\n{text}\r\n").into_bytes());
    if ends.frames.send(frame).await.is_err() {
        tracing::debug!("the terminal closed before its notice");
    }
}

fn header(value: &str) -> Result<HeaderValue> {
    HeaderValue::from_str(value).map_err(stream_error)
}

fn stream_error(error: impl std::fmt::Display) -> Error {
    Error::Stream(error.to_string())
}

#[cfg(test)]
mod tests {
    use std::sync::Mutex;

    use async_channel::unbounded;
    use futures_util::SinkExt as _;
    use protocol::{DaemonRequest, DaemonResponse, RelayEndpoint, Responder};
    use tokio::net::TcpListener;
    use tokio::sync::Notify;
    use tokio_tungstenite::accept_hdr_async;
    use tokio_tungstenite::tungstenite::Message;
    use tokio_tungstenite::tungstenite::handshake::server::{ErrorResponse, Request, Response};

    use super::*;

    const CREATED: u16 = 201;

    struct Daemon {
        relay: String,
        seen: Mutex<Vec<String>>,
        deleted: Notify,
    }

    fn note_delete(
        daemon: &Daemon,
        request: &DaemonRequest,
    ) {
        if request.method().as_str() != "DELETE" {
            return;
        }
        daemon.deleted.notify_one();
    }

    impl DaemonTransport for Daemon {
        fn request(
            &self,
            request: DaemonRequest,
            respond: Responder,
        ) {
            let line = format!(
                "{} {}",
                request.method().as_str(),
                request.target().as_str()
            );
            note_delete(self, &request);
            self.seen.lock().unwrap().push(line);
            let body = r#"{"success":true,"data":{"sessionId":"s1"}}"#;
            respond(Ok(DaemonResponse::new(CREATED, Vec::new(), body.into())));
        }

        fn relay_endpoint(
            &self,
            _scope: &RelayScope,
        ) -> protocol::Result<RelayEndpoint> {
            Ok(RelayEndpoint::new(self.relay.clone(), "tok"))
        }
    }

    #[allow(
        clippy::result_large_err,
        clippy::unnecessary_wraps,
        reason = "the signature is fixed by tungstenite's Callback"
    )]
    fn select_relay(
        _request: &Request,
        mut response: Response,
    ) -> std::result::Result<Response, ErrorResponse> {
        let selected = HeaderValue::from_static(RELAY_SUBPROTOCOL);
        response
            .headers_mut()
            .insert("sec-websocket-protocol", selected);
        Ok(response)
    }

    fn sessions(daemon: &Arc<Daemon>) -> TerminalSessions {
        let transport: Arc<dyn DaemonTransport> = Arc::<Daemon>::clone(daemon);
        let host = HostId::new("local").unwrap();
        TerminalSessions::new(transport, host, Handle::current(), "crowbar://local")
    }

    async fn serve_output(listener: TcpListener) {
        let (tcp, _) = listener.accept().await.unwrap();
        let mut socket = accept_hdr_async(tcp, select_relay).await.unwrap();
        let output = Message::binary(vec![0, b'o', b'k']);
        socket.send(output).await.unwrap();
        let exit = Message::text(r#"{"type":"exit","code":0}"#);
        socket.send(exit).await.unwrap();
        socket.close(None).await.unwrap();
    }

    async fn serve_then_drop(listener: TcpListener) {
        let (tcp, _) = listener.accept().await.unwrap();
        let mut socket = accept_hdr_async(tcp, select_relay).await.unwrap();
        socket.close(None).await.unwrap();
        serve_output(listener).await;
    }

    async fn serve_held(listener: TcpListener) {
        let (tcp, _) = listener.accept().await.unwrap();
        let mut socket = accept_hdr_async(tcp, select_relay).await.unwrap();
        let output = Message::binary(vec![0, b'o', b'k']);
        socket.send(output).await.unwrap();
        while socket.next().await.is_some_and(|message| message.is_ok()) {}
    }

    fn daemon_on(listener: &TcpListener) -> Arc<Daemon> {
        Arc::new(Daemon {
            relay: format!("ws://{}", listener.local_addr().unwrap()),
            seen: Mutex::default(),
            deleted: Notify::new(),
        })
    }

    async fn first_output(frames: &Receiver<DaemonFrame>) -> String {
        let mut text = String::new();
        while !text.contains("ok") {
            text.push_str(&notice_text(frames).await);
        }
        text
    }

    async fn until_closed(frames: &Receiver<DaemonFrame>) {
        while frames.recv().await.is_ok() {}
    }

    async fn notice_text(frames: &Receiver<DaemonFrame>) -> String {
        let frame = frames.recv().await.unwrap();
        let DaemonFrame::Output(bytes) = frame else {
            panic!("expected a notice");
        };
        String::from_utf8(bytes).unwrap()
    }

    const CASES: [&str; 1] = ["agent"];

    #[tokio::test]
    async fn a_session_is_created_streamed_and_killed() {
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let relay = format!("ws://{}", listener.local_addr().unwrap());
        let daemon = Arc::new(Daemon {
            relay,
            seen: Mutex::default(),
            deleted: Notify::new(),
        });
        let server = tokio::spawn(serve_output(listener));
        let (frames, view_frames) = unbounded();
        let (_view_messages, messages) = unbounded();
        let base = SessionBase::chat("c1").unwrap();
        let pane = PaneId::new("t1").unwrap();
        sessions(&daemon).start(base, pane, Ends { frames, messages });
        let first = view_frames.recv().await.unwrap();
        assert_eq!(first, DaemonFrame::Output(b"ok".to_vec()));
        server.await.unwrap();
        until_closed(&view_frames).await;
        let seen = daemon.seen.lock().unwrap().clone();
        assert_eq!(
            seen,
            [
                "POST /v0/chats/c1/terminals",
                "DELETE /v0/chats/c1/terminals/s1"
            ]
        );
    }

    #[tokio::test]
    async fn a_dropped_stream_reattaches_to_the_same_session_without_killing_it() {
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let daemon = daemon_on(&listener);
        let server = tokio::spawn(serve_then_drop(listener));
        let (frames, view_frames) = unbounded();
        let (_view_messages, messages) = unbounded();
        let base = SessionBase::chat("c1").unwrap();
        let pane = PaneId::new("t1").unwrap();
        sessions(&daemon).start(base, pane, Ends { frames, messages });
        let shown = first_output(&view_frames).await;
        assert!(shown.contains("reconnecting"), "{shown}");
        server.await.unwrap();
        daemon.deleted.notified().await;
        let seen = daemon.seen.lock().unwrap().clone();
        assert_eq!(
            seen,
            [
                "POST /v0/chats/c1/terminals",
                "DELETE /v0/chats/c1/terminals/s1"
            ]
        );
    }

    #[tokio::test]
    async fn closing_the_view_kills_the_session_it_created() {
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let daemon = daemon_on(&listener);
        let server = tokio::spawn(serve_held(listener));
        let (frames, view_frames) = unbounded();
        let (view_messages, messages) = unbounded();
        let base = SessionBase::chat("c1").unwrap();
        let pane = PaneId::new("t1").unwrap();
        sessions(&daemon).start(base, pane, Ends { frames, messages });
        first_output(&view_frames).await;
        drop(view_frames);
        drop(view_messages);
        daemon.deleted.notified().await;
        server.await.unwrap();
        let seen = daemon.seen.lock().unwrap().clone();
        assert_eq!(seen.last().unwrap(), "DELETE /v0/chats/c1/terminals/s1");
    }

    #[tokio::test]
    async fn a_session_that_cannot_connect_says_so_in_the_terminal() {
        let daemon = Arc::new(Daemon {
            relay: "ws://127.0.0.1:1".into(),
            seen: Mutex::default(),
            deleted: Notify::new(),
        });
        let (frames, view_frames) = unbounded();
        let (_view_messages, messages) = unbounded();
        let base = SessionBase::chat("c1").unwrap();
        let pane = PaneId::new("t1").unwrap();
        sessions(&daemon).start(base, pane, Ends { frames, messages });
        let notice = notice_text(&view_frames).await;
        assert!(notice.contains("stream"), "names the stage");
    }

    #[tokio::test]
    async fn watching_an_existing_session_creates_and_kills_nothing() {
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let relay = format!("ws://{}", listener.local_addr().unwrap());
        let daemon = Arc::new(Daemon {
            relay,
            seen: Mutex::default(),
            deleted: Notify::new(),
        });
        let server = tokio::spawn(serve_output(listener));
        let (frames, view_frames) = unbounded();
        let (_view_messages, messages) = unbounded();
        let base = SessionBase::chat("c1").unwrap();
        let pane = PaneId::new("t1").unwrap();
        sessions(&daemon).watch((base, CASES[0].into()), pane, Ends { frames, messages });
        let first = view_frames.recv().await.unwrap();
        assert_eq!(first, DaemonFrame::Output(b"ok".to_vec()));
        server.await.unwrap();
        until_closed(&view_frames).await;
        assert!(daemon.seen.lock().unwrap().is_empty(), "no REST calls");
    }
}
