//! One terminal session from creation to the end of its stream.

use std::sync::Arc;

use protocol::{DaemonTransport, HostId, PaneId, PaneOrigin, RelayScope};
use tokio::runtime::Handle;
use tokio::time::timeout;
use tokio_tungstenite::client_async;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;

use crate::ends::Ends;
use crate::error::{Error, Result};
use crate::rest::Binding;
use crate::rest::Rest;
use crate::rest::SessionPaths;
use crate::stream::Attachment;
use crate::stream::End;
use crate::stream::Reconnect;
use crate::stream::pump;
use crate::stream::{CONNECT_TIMEOUT, header, stream_error, within};
use crate::stream::{Dial, tcp_dial};
use crate::stream::{notice, recover};

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
    dial: Dial,
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
        Self::dialing(
            Rest { transport, host },
            runtime,
            origin,
            Arc::new(tcp_dial),
        )
    }

    pub(crate) fn dialing(
        rest: Rest,
        runtime: Handle,
        origin: &str,
        dial: Dial,
    ) -> Self {
        Self {
            rest,
            runtime,
            origin: origin.to_owned(),
            dial,
        }
    }

    /// Creates a session in `base` for `pane` and connects it to `ends`. A failure is written
    /// into the terminal itself; a dropped connection reattaches to the same PTY; dropping the view ends the session and kills it.
    pub fn start(
        &self,
        base: SessionPaths,
        pane: PaneId,
        ends: Ends,
    ) {
        self.spawn((base, Binding::Create), pane, ends);
    }

    /// Connects `ends` to the existing `session` of `base` without creating or killing anything,
    /// for the PTY of an agent runner.
    pub fn watch(
        &self,
        (base, session): (SessionPaths, String),
        pane: PaneId,
        ends: Ends,
    ) {
        self.spawn((base, Binding::Existing(session)), pane, ends);
    }

    fn spawn(
        &self,
        (base, binding): (SessionPaths, Binding),
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
        (base, binding): (&SessionPaths, &Binding),
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
        (base, binding): (&SessionPaths, &Binding),
        scope: &RelayScope,
        ends: &Ends,
    ) -> Result<()> {
        let created = timeout(CONNECT_TIMEOUT, binding.session(&self.rest, base));
        let session = created.await.map_err(|source| Error::TimedOut {
            stage: "creating the session",
            source,
        })??;
        let attachment = Attachment {
            scope: scope.clone(),
            ends: ends.clone(),
        };
        self.keep_attached((base, &session), &attachment).await;
        binding.release(&self.rest, base, &session).await;
        Ok(())
    }

    async fn keep_attached(
        &self,
        target: (&SessionPaths, &str),
        attachment: &Attachment,
    ) {
        let mut reconnect = Reconnect::default();
        while !self.round(target, attachment, &mut reconnect).await {}
    }

    async fn round(
        &self,
        target: (&SessionPaths, &str),
        attachment: &Attachment,
        reconnect: &mut Reconnect,
    ) -> bool {
        let outcome = self.stream(target, attachment).await;
        let reason = match outcome {
            Ok(End::ViewGone | End::Exited) => return true,
            Ok(End::Lost) => {
                reconnect.connected();
                "the connection dropped".to_owned()
            },
            Err(error) => error.to_string(),
        };
        tracing::warn!(%reason, "terminal stream lost");
        !recover(&attachment.ends, reconnect, &reason).await
    }

    async fn stream(
        &self,
        (base, session): (&SessionPaths, &str),
        attachment: &Attachment,
    ) -> Result<End> {
        let endpoint = self
            .rest
            .transport
            .relay_endpoint(&attachment.scope)
            .map_err(stream_error)?;
        let address = endpoint.url().trim_start_matches("ws://").to_owned();
        let tcp = within((self.dial)(address)).await?;
        let url = format!("{}{}", endpoint.url(), base.stream_path(session));
        let mut request = url.into_client_request().map_err(stream_error)?;
        let protocols = format!("{RELAY_SUBPROTOCOL}, {TOKEN_PREFIX}{}", endpoint.token());
        let headers = request.headers_mut();
        headers.insert("sec-websocket-protocol", header(&protocols)?);
        headers.insert("origin", header(&self.origin)?);
        let (socket, _) = within(client_async(request, tcp)).await?;
        Ok(pump(socket, attachment.ends.clone()).await)
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Mutex;

    use async_channel::{Receiver, unbounded};
    use futures_util::SinkExt as _;
    use futures_util::StreamExt;
    use protocol::{DaemonRequest, DaemonResponse, RelayEndpoint, Responder};
    use terminal::DaemonFrame;
    use tokio::io::DuplexStream;
    use tokio::sync::Notify;
    use tokio::sync::mpsc::UnboundedReceiver;
    use tokio_tungstenite::accept_hdr_async;
    use tokio_tungstenite::tungstenite::Message;
    use tokio_tungstenite::tungstenite::handshake::server::{
        Callback, ErrorResponse, Request, Response,
    };
    use tokio_tungstenite::tungstenite::http::HeaderValue;

    use super::*;
    use crate::test_support::memory_dial::{memory_dial, refusing_dial};

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

    struct SelectRelay;

    impl Callback for SelectRelay {
        fn on_request(
            self,
            _request: &Request,
            mut response: Response,
        ) -> std::result::Result<Response, ErrorResponse> {
            let selected = HeaderValue::from_static(RELAY_SUBPROTOCOL);
            response
                .headers_mut()
                .insert("sec-websocket-protocol", selected);
            Ok(response)
        }
    }

    type Accepted = UnboundedReceiver<DuplexStream>;

    fn sessions(
        daemon: &Arc<Daemon>,
        dial: Dial,
    ) -> TerminalSessions {
        let transport: Arc<dyn DaemonTransport> = Arc::<Daemon>::clone(daemon);
        let host = HostId::new("local").unwrap();
        TerminalSessions::dialing(
            Rest { transport, host },
            Handle::current(),
            "crowbar://local",
            dial,
        )
    }

    async fn serve_output(accepted: &mut Accepted) {
        let tcp = accepted.recv().await.unwrap();
        let mut socket = accept_hdr_async(tcp, SelectRelay).await.unwrap();
        let output = Message::binary(vec![0, b'o', b'k']);
        socket.send(output).await.unwrap();
        let exit = Message::text(r#"{"type":"exit","code":0}"#);
        socket.send(exit).await.unwrap();
        socket.close(None).await.unwrap();
    }

    async fn serve_then_drop(mut accepted: Accepted) {
        let tcp = accepted.recv().await.unwrap();
        let mut socket = accept_hdr_async(tcp, SelectRelay).await.unwrap();
        socket.close(None).await.unwrap();
        serve_output(&mut accepted).await;
    }

    async fn serve_held(mut accepted: Accepted) {
        let tcp = accepted.recv().await.unwrap();
        let mut socket = accept_hdr_async(tcp, SelectRelay).await.unwrap();
        let output = Message::binary(vec![0, b'o', b'k']);
        socket.send(output).await.unwrap();
        while socket.next().await.is_some_and(|message| message.is_ok()) {}
    }

    fn daemon() -> Arc<Daemon> {
        Arc::new(Daemon {
            relay: "ws://relay".into(),
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

    async fn notice_text(frames: &Receiver<DaemonFrame>) -> String {
        let frame = frames.recv().await.unwrap();
        let DaemonFrame::Output(bytes) = frame else {
            panic!("expected a notice");
        };
        String::from_utf8(bytes).unwrap()
    }

    #[derive(Clone, Copy)]
    enum Relay {
        Output,
        DropsOnce,
    }

    struct Case {
        name: &'static str,
        relay: Relay,
        watching: Option<&'static str>,
        shown: &'static str,
        rest_calls: &'static [&'static str],
    }

    const CREATE: &[&str] = &[
        "POST /v0/chats/c1/terminals",
        "DELETE /v0/chats/c1/terminals/s1",
    ];

    const CASES: &[Case] = &[
        Case {
            name: "a session is created, streamed and killed",
            relay: Relay::Output,
            watching: None,
            shown: "ok",
            rest_calls: CREATE,
        },
        Case {
            name: "a dropped stream reattaches to the same session without killing it",
            relay: Relay::DropsOnce,
            watching: None,
            shown: "reconnecting",
            rest_calls: CREATE,
        },
        Case {
            name: "watching an existing session creates and kills nothing",
            relay: Relay::Output,
            watching: Some("agent"),
            shown: "ok",
            rest_calls: &[],
        },
    ];

    async fn all_frames(frames: &Receiver<DaemonFrame>) -> String {
        let mut text = String::new();
        while let Ok(DaemonFrame::Output(bytes)) = frames.recv().await {
            text.push_str(&String::from_utf8_lossy(&bytes));
        }
        text
    }

    async fn run_case(case: &Case) {
        let (dial, accepted) = memory_dial();
        let daemon = daemon();
        let server = match case.relay {
            Relay::Output => tokio::spawn(async move {
                let mut accepted = accepted;
                serve_output(&mut accepted).await;
            }),
            Relay::DropsOnce => tokio::spawn(serve_then_drop(accepted)),
        };
        let (frames, view_frames) = unbounded();
        let (_view_messages, messages) = unbounded();
        let base = SessionPaths::chat("c1").unwrap();
        let pane = PaneId::new("t1").unwrap();
        let ends = Ends { frames, messages };
        match case.watching {
            Some(session) => sessions(&daemon, dial).watch((base, session.into()), pane, ends),
            None => sessions(&daemon, dial).start(base, pane, ends),
        }
        let shown = all_frames(&view_frames).await;
        server.await.unwrap();
        assert!(shown.contains(case.shown), "{}: {shown}", case.name);
        let seen = daemon.seen.lock().unwrap().clone();
        assert_eq!(seen, case.rest_calls, "{}", case.name);
    }

    #[tokio::test]
    async fn sessions_report_their_rest_calls_and_output() {
        run_all().await;
    }

    async fn run_all() {
        for case in CASES {
            run_case(case).await;
        }
    }

    #[tokio::test]
    async fn closing_the_view_kills_the_session_it_created() {
        let (dial, accepted) = memory_dial();
        let daemon = daemon();
        let server = tokio::spawn(serve_held(accepted));
        let (frames, view_frames) = unbounded();
        let (view_messages, messages) = unbounded();
        let base = SessionPaths::chat("c1").unwrap();
        let pane = PaneId::new("t1").unwrap();
        sessions(&daemon, dial).start(base, pane, Ends { frames, messages });
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
        let daemon = daemon();
        let (frames, view_frames) = unbounded();
        let (_view_messages, messages) = unbounded();
        let base = SessionPaths::chat("c1").unwrap();
        let pane = PaneId::new("t1").unwrap();
        sessions(&daemon, refusing_dial()).start(base, pane, Ends { frames, messages });
        let notice = notice_text(&view_frames).await;
        assert!(notice.contains("stream"), "names the stage");
    }
}
