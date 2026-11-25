//! One terminal session from creation to the end of its stream.

use std::sync::Arc;

use async_channel::Sender;
use protocol::{DaemonTransport, HostId, PaneOrigin, RelayScope};
use tokio::runtime::Handle;
use tokio::time::timeout;
use tokio_tungstenite::client_async;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;

use crate::connection::Connection;
use crate::error::{Error, Result};
use crate::readiness::Readiness;
use crate::report::Report;
use crate::rest::Binding;
use crate::rest::Rest;
use crate::rest::SessionPaths;
use crate::stream::Attachment;
use crate::stream::End;
use crate::stream::Reconnect;
use crate::stream::pump;
use crate::stream::recover;
use crate::stream::{CONNECT_TIMEOUT, header, stream_error, within};
use crate::stream::{Dial, tcp_dial};

const RELAY_SUBPROTOCOL: &str = "crowbar.relay.v1";
const TOKEN_PREFIX: &str = "crowbar.token.";

/// Starts terminal sessions on one daemon host.
#[derive(Clone)]
pub struct TerminalSessions {
    rest: Rest,
    runtime: Handle,
    origin: String,
    dial: Dial,
    ready: Readiness,
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

    /// The same sessions, asked of `host` instead.
    #[must_use]
    pub fn on(
        &self,
        host: HostId,
    ) -> Self {
        let mut sessions = self.clone();
        sessions.rest.host = host;
        sessions.ready = Readiness::immediate();
        sessions
    }

    /// The same sessions, binding only once `ready` says the daemon serves.
    #[must_use]
    pub fn once(
        self,
        ready: Readiness,
    ) -> Self {
        Self { ready, ..self }
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
            ready: Readiness::immediate(),
        }
    }

    /// Binds `connection`'s pane to a session of `base` as `binding` says, reports how it went
    /// and streams it; a dropped connection reattaches to the same PTY, and dropping the view ends
    /// the stream and kills the session unless it was only watched.
    pub fn open(
        &self,
        base: SessionPaths,
        binding: Binding,
        connection: Connection,
    ) {
        let session = self.clone();
        self.runtime.spawn(async move {
            session.run((&base, &binding), connection).await;
        });
    }

    async fn run(
        &self,
        (base, binding): (&SessionPaths, &Binding),
        connection: Connection,
    ) {
        let Connection {
            pane,
            ends,
            reports,
        } = connection;
        let scope = RelayScope::new(self.rest.host.clone(), pane, PaneOrigin::Core);
        let attachment = Attachment { scope, ends };
        self.ready.wait().await;
        if attachment.ends.messages.is_closed() {
            return;
        }
        if let Err(error) = self.attach((base, binding), &attachment, &reports).await {
            report_failure(&reports, &error).await;
        }
        self.rest.transport.revoke_pane(&attachment.scope);
    }

    async fn attach(
        &self,
        (base, binding): (&SessionPaths, &Binding),
        attachment: &Attachment,
        reports: &Sender<Report>,
    ) -> Result<()> {
        let resolved = timeout(CONNECT_TIMEOUT, binding.session(&self.rest, base));
        let session = resolved.await.map_err(|source| Error::TimedOut {
            stage: "creating the session",
            source,
        })??;
        let Some(session) = session else {
            report(reports, Report::Gone).await;
            return Ok(());
        };
        if matches!(binding, Binding::Create) {
            report(reports, Report::Bound(session.clone())).await;
        }
        let end = self.keep_attached((base, &session), attachment).await;
        let released = self.rest.kill(base, &session).await;
        if let (End::ViewGone, Err(error)) = (end, released) {
            report(reports, Report::StopFailed(error.words())).await;
        }
        Ok(())
    }

    async fn keep_attached(
        &self,
        target: (&SessionPaths, &str),
        attachment: &Attachment,
    ) -> End {
        let mut reconnect = Reconnect::default();
        let mut ended = None;
        while ended.is_none() {
            ended = self.round(target, attachment, &mut reconnect).await;
        }
        ended.unwrap_or(End::Lost)
    }

    async fn round(
        &self,
        target: (&SessionPaths, &str),
        attachment: &Attachment,
        reconnect: &mut Reconnect,
    ) -> Option<End> {
        let outcome = self.stream(target, attachment).await;
        let reason = match outcome {
            Ok(End::Lost) => {
                reconnect.connected();
                "the connection dropped".to_owned()
            },
            Ok(end) => return Some(end),
            Err(error) => error.to_string(),
        };
        warn_stream_lost(&reason);
        let recovered = recover(&attachment.ends, reconnect, &reason).await;
        (!recovered).then_some(End::Lost)
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

async fn report_failure(
    reports: &Sender<Report>,
    error: &Error,
) {
    tracing::warn!(%error, "terminal session failed");
    report(reports, Report::Failed(error.to_string())).await;
}

fn warn_stream_lost(reason: &str) {
    tracing::warn!(%reason, "terminal stream lost");
}

async fn report(
    reports: &Sender<Report>,
    turn: Report,
) {
    if reports.send(turn).await.is_err() {
        tracing::debug!("the host stopped listening to a terminal session");
    }
}

#[cfg(test)]
mod tests {
    use async_channel::{Receiver, unbounded};
    use terminal::DaemonFrame;

    use protocol::PaneId;

    use super::*;
    use crate::ends::Ends;
    use crate::test_support::{
        CREATED, FAILED, FakeDaemon, KILL_REFUSED, LISTED, NONE_LISTED, daemon, daemon_answering,
        daemon_refusing_kills, memory_dial, refusing_dial, serve_held, serve_output,
        serve_then_drop,
    };

    fn sessions(
        daemon: &Arc<FakeDaemon>,
        dial: Dial,
    ) -> TerminalSessions {
        let transport: Arc<dyn DaemonTransport> = Arc::<FakeDaemon>::clone(daemon);
        let host = HostId::new("local").unwrap();
        TerminalSessions::dialing(
            Rest { transport, host },
            Handle::current(),
            "crowbar://local",
            dial,
        )
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

    #[derive(Clone, Copy)]
    enum Bind {
        Create,
        Resume(&'static str),
    }

    impl Bind {
        fn binding(self) -> Binding {
            match self {
                Self::Create => Binding::Create,
                Self::Resume(session) => Binding::Saved(session.into()),
            }
        }
    }

    struct Case {
        name: &'static str,
        relay: Relay,
        bind: Bind,
        rest_calls: &'static [&'static str],
        first_report: Option<Report>,
    }

    const CREATE: &[&str] = &[
        "POST /v0/chats/c1/terminals",
        "DELETE /v0/chats/c1/terminals/s1",
    ];

    const RESUME: &[&str] = &[
        "GET /v0/chats/c1/terminals",
        "DELETE /v0/chats/c1/terminals/s1",
    ];

    const CASES: &[Case] = &[
        Case {
            name: "a session is created, reported, streamed and killed",
            relay: Relay::Output,
            bind: Bind::Create,
            rest_calls: CREATE,
            first_report: Some(Report::Bound(String::new())),
        },
        Case {
            name: "a dropped stream reattaches to the same session without killing it",
            relay: Relay::DropsOnce,
            bind: Bind::Create,
            rest_calls: CREATE,
            first_report: Some(Report::Bound(String::new())),
        },
        Case {
            name: "a saved session the daemon still lists is attached, not created",
            relay: Relay::Output,
            bind: Bind::Resume("s1"),
            rest_calls: RESUME,
            first_report: None,
        },
    ];

    fn bound(report: Option<Report>) -> Option<Report> {
        report.map(|report| match report {
            Report::Bound(_) => Report::Bound("s1".into()),
            other => other,
        })
    }

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
        let (reports, heard) = unbounded();
        let base = SessionPaths::chat("c1").unwrap();
        let pane = PaneId::new("t1").unwrap();
        let connection = Connection {
            pane,
            ends: Ends { frames, messages },
            reports,
        };
        sessions(&daemon, dial).open(base, case.bind.binding(), connection);
        let shown = all_frames(&view_frames).await;
        server.await.unwrap();
        assert_eq!(shown, "ok", "{}", case.name);
        let seen = daemon.seen.lock().unwrap().clone();
        assert_eq!(seen, case.rest_calls, "{}", case.name);
        assert_eq!(
            heard.try_recv().ok(),
            bound(case.first_report.clone()),
            "{}",
            case.name
        );
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

    async fn close_the_view(daemon: &Arc<FakeDaemon>) -> Vec<Report> {
        let (dial, accepted) = memory_dial();
        let server = tokio::spawn(serve_held(accepted));
        let (frames, view_frames) = unbounded();
        let (view_messages, messages) = unbounded();
        let (reports, heard) = unbounded();
        let connection = Connection {
            pane: PaneId::new("t1").unwrap(),
            ends: Ends { frames, messages },
            reports,
        };
        let base = SessionPaths::chat("c1").unwrap();
        sessions(daemon, dial).open(base, Binding::Create, connection);
        first_output(&view_frames).await;
        drop((view_frames, view_messages));
        server.await.unwrap();
        let mut told = Vec::new();
        while let Ok(report) = heard.recv().await {
            told.push(report);
        }
        let seen = daemon.seen.lock().unwrap().clone();
        assert_eq!(seen.last().unwrap(), "DELETE /v0/chats/c1/terminals/s1");
        told
    }

    #[tokio::test]
    async fn closing_the_view_kills_the_session_it_created_and_reports_a_refused_kill() {
        let bound = || Report::Bound("s1".into());
        assert_eq!(close_the_view(&daemon()).await, [bound()]);
        let refused = close_the_view(&daemon_refusing_kills()).await;
        let stop_failed = Report::StopFailed(KILL_REFUSED.into());
        assert_eq!(refused, [bound(), stop_failed]);
    }

    async fn failed_start(
        daemon: &Arc<FakeDaemon>,
        bind: Bind,
    ) -> (String, Option<Report>) {
        let (frames, view_frames) = unbounded();
        let (_view_messages, messages) = unbounded();
        let (reports, heard) = unbounded();
        let base = SessionPaths::chat("c1").unwrap();
        let pane = PaneId::new("t1").unwrap();
        let connection = Connection {
            pane,
            ends: Ends { frames, messages },
            reports,
        };
        sessions(daemon, refusing_dial()).open(base, bind.binding(), connection);
        let report = heard.recv().await.ok();
        (all_frames(&view_frames).await, report)
    }

    #[tokio::test]
    async fn a_session_the_daemon_cannot_create_is_reported_not_written_into_the_terminal() {
        let daemon = daemon_answering(FAILED, LISTED);
        let (shown, report) = failed_start(&daemon, Bind::Create).await;
        assert_eq!(shown, "", "nothing written into the terminal");
        let reason = failure_reason(report);
        assert!(reason.contains("500"), "{reason}");
    }

    fn failure_reason(report: Option<Report>) -> String {
        let Some(Report::Failed(reason)) = report else {
            panic!("expected a failure, got {report:?}");
        };
        reason
    }

    #[tokio::test]
    async fn a_saved_session_the_daemon_no_longer_lists_is_gone_and_none_is_made() {
        let daemon = daemon_answering(CREATED, NONE_LISTED);
        let (shown, report) = failed_start(&daemon, Bind::Resume("s1")).await;
        assert_eq!((shown.as_str(), report), ("", Some(Report::Gone)));
        let seen = daemon.seen.lock().unwrap().clone();
        assert_eq!(
            seen,
            ["GET /v0/chats/c1/terminals"],
            "no session made or killed"
        );
    }
}
