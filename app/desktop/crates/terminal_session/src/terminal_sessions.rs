//! One terminal session from creation to the end of its stream.

use std::sync::Arc;

use protocol::{DaemonTransport, HostId, PaneId, PaneOrigin, RelayScope};
use terminal::DaemonFrame;
use tokio::net::TcpStream;
use tokio::runtime::Handle;
use tokio_tungstenite::client_async;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;
use tokio_tungstenite::tungstenite::http::HeaderValue;

use crate::binding::Binding;
use crate::ends::Ends;
use crate::error::{Error, Result};
use crate::rest::Rest;
use crate::session_base::SessionBase;
use crate::stream::pump;

const RELAY_SUBPROTOCOL: &str = "crowbar.relay.v1";
const TOKEN_PREFIX: &str = "crowbar.token.";
const FAILURE_COLOR: &str = "\x1b[31m";
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
    /// into the terminal itself; dropping the view ends the session and kills the PTY.
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
        let notice = ends.frames.clone();
        if let Err(error) = self.attach((base, binding), &scope, ends).await {
            tracing::warn!(%error, "terminal session failed");
            let text = format!("{FAILURE_COLOR}{error}{RESET}\r\n");
            let reported = notice.send(DaemonFrame::Output(text.into_bytes())).await;
            reported.unwrap_or_default();
        }
        self.rest.transport.revoke_pane(&scope);
    }

    async fn attach(
        &self,
        (base, binding): (&SessionBase, &Binding),
        scope: &RelayScope,
        ends: Ends,
    ) -> Result<()> {
        let session = binding.session(&self.rest, base).await?;
        let outcome = self.stream(base, &session, scope, ends).await;
        binding.release(&self.rest, base, &session).await;
        outcome
    }

    async fn stream(
        &self,
        base: &SessionBase,
        session: &str,
        scope: &RelayScope,
        ends: Ends,
    ) -> Result<()> {
        let endpoint = self
            .rest
            .transport
            .relay_endpoint(scope)
            .map_err(stream_error)?;
        let address = endpoint.url().trim_start_matches("ws://").to_owned();
        let tcp = TcpStream::connect(&address).await.map_err(stream_error)?;
        let url = format!("{}{}", endpoint.url(), base.stream_path(session));
        let mut request = url.into_client_request().map_err(stream_error)?;
        let protocols = format!("{RELAY_SUBPROTOCOL}, {TOKEN_PREFIX}{}", endpoint.token());
        let headers = request.headers_mut();
        headers.insert("sec-websocket-protocol", header(&protocols)?);
        headers.insert("origin", header(&self.origin)?);
        let (socket, _) = client_async(request, tcp).await.map_err(stream_error)?;
        pump(socket, ends).await;
        Ok(())
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
    use tokio_tungstenite::accept_hdr_async;
    use tokio_tungstenite::tungstenite::Message;
    use tokio_tungstenite::tungstenite::handshake::server::{ErrorResponse, Request, Response};

    use super::*;

    const CREATED: u16 = 201;

    struct Daemon {
        relay: String,
        seen: Mutex<Vec<String>>,
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
        socket.close(None).await.unwrap();
    }

    async fn until_closed(frames: &async_channel::Receiver<DaemonFrame>) {
        while frames.recv().await.is_ok() {}
    }

    async fn notice_text(frames: &async_channel::Receiver<DaemonFrame>) -> String {
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
    async fn a_session_that_cannot_connect_says_so_in_the_terminal() {
        let daemon = Arc::new(Daemon {
            relay: "ws://127.0.0.1:1".into(),
            seen: Mutex::default(),
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
