mod staying_attached;
mod terminal_session_event;

use std::sync::Arc;

use async_channel::Sender;
use protocol::{DaemonTransport, HostId, PaneOrigin, RelayScope};
use tokio::runtime::Handle;
use tokio::time::timeout;

use crate::connection::Connection;
use crate::error::{Error, Result};
use crate::journal_sink::emit;
use crate::readiness::Readiness;
use crate::report::Report;
use crate::rest::Binding;
use crate::rest::Rest;
use crate::rest::SessionPaths;
use crate::stream::Attachment;
use crate::stream::CONNECT_TIMEOUT;
use crate::stream::End;
use crate::stream::{Dial, tcp_dial};
use crate::terminal_sessions::terminal_session_event::TerminalSessionEvent;

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
            emit(&TerminalSessionEvent::Failed { error: &error });
            report(&reports, Report::Failed(error.to_string())).await;
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
        if end == End::Gone {
            report(reports, Report::Gone).await;
            return Ok(());
        }
        let released = self.rest.kill(base, &session).await;
        if let (End::ViewGone, Err(error)) = (end, released) {
            report(reports, Report::StopFailed(error.words())).await;
        }
        Ok(())
    }
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
    use terminal::DaemonFrame;

    use super::*;
    use crate::test_support::{
        CREATED, FAILED, FakeDaemon, KILL_REFUSED, LISTED, NONE_LISTED, View, all_reports, daemon,
        daemon_answering, daemon_refusing_kills, memory_dial, refusing_dial, serve_held,
        serve_output, serve_then_drop, sessions,
    };

    #[derive(Clone, Copy)]
    enum Relay {
        Output,
        DropsOnce,
    }

    struct Case {
        name: &'static str,
        relay: Relay,
        saved: Option<&'static str>,
        rest_calls: &'static [&'static str],
        first_report: Option<&'static str>,
    }

    const CASES: &[Case] = &[
        Case {
            name: "a session is created, reported, streamed and killed",
            relay: Relay::Output,
            saved: None,
            rest_calls: &[
                "POST /v0/chats/c1/terminals",
                "DELETE /v0/chats/c1/terminals/s1",
            ],
            first_report: Some("s1"),
        },
        Case {
            name: "a dropped stream reattaches to the same session, still listed, without killing it",
            relay: Relay::DropsOnce,
            saved: None,
            rest_calls: &[
                "POST /v0/chats/c1/terminals",
                "GET /v0/chats/c1/terminals",
                "DELETE /v0/chats/c1/terminals/s1",
            ],
            first_report: Some("s1"),
        },
        Case {
            name: "a saved session the daemon still lists is attached, not created",
            relay: Relay::Output,
            saved: Some("s1"),
            rest_calls: &[
                "GET /v0/chats/c1/terminals",
                "DELETE /v0/chats/c1/terminals/s1",
            ],
            first_report: None,
        },
    ];

    fn binding(saved: Option<&str>) -> Binding {
        saved.map_or(Binding::Create, |session| Binding::Saved(session.into()))
    }

    async fn run_case(case: &Case) {
        let (dial, mut accepted) = memory_dial();
        let daemon = daemon();
        let server = match case.relay {
            Relay::Output => tokio::spawn(async move { serve_output(&mut accepted).await }),
            Relay::DropsOnce => tokio::spawn(serve_then_drop(accepted)),
        };
        let view = View::open(&sessions(&daemon, dial), binding(case.saved));
        let shown = view.all_output().await;
        server.await.unwrap();
        assert_eq!(shown, "ok", "{}", case.name);
        let seen = daemon.seen.lock().unwrap().clone();
        assert_eq!(seen, case.rest_calls, "{}", case.name);
        let bound = case.first_report.map(|id| Report::Bound(id.into()));
        assert_eq!(view.reports.try_recv().ok(), bound, "{}", case.name);
    }

    async fn run_all() {
        for case in CASES {
            run_case(case).await;
        }
    }

    #[tokio::test]
    async fn sessions_report_their_rest_calls_and_output() {
        run_all().await;
    }

    async fn close_the_view(daemon: &Arc<FakeDaemon>) -> Vec<Report> {
        let (dial, accepted) = memory_dial();
        let server = tokio::spawn(serve_held(accepted));
        let view = View::open(&sessions(daemon, dial), Binding::Create);
        let first = view.frames.recv().await.unwrap();
        assert!(matches!(first, DaemonFrame::Output(_)), "{first:?}");
        let View {
            frames,
            messages,
            reports,
        } = view;
        drop((frames, messages));
        server.await.unwrap();
        let seen = daemon.seen.lock().unwrap().clone();
        assert_eq!(seen.last().unwrap(), "DELETE /v0/chats/c1/terminals/s1");
        all_reports(&reports).await
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
        saved: Option<&str>,
    ) -> (String, Option<Report>) {
        let view = View::open(&sessions(daemon, refusing_dial()), binding(saved));
        let report = view.reports.recv().await.ok();
        (view.all_output().await, report)
    }

    #[tokio::test]
    async fn a_session_the_daemon_cannot_create_is_reported_not_written_into_the_terminal() {
        let daemon = daemon_answering(FAILED, LISTED);
        let (shown, report) = failed_start(&daemon, None).await;
        assert_eq!(shown, "", "nothing written into the terminal");
        let failed = matches!(&report, Some(Report::Failed(reason)) if reason.contains("500"));
        assert!(failed, "{report:?}");
    }

    #[tokio::test]
    async fn a_saved_session_the_daemon_no_longer_lists_is_gone_and_none_is_made() {
        let daemon = daemon_answering(CREATED, NONE_LISTED);
        let (shown, report) = failed_start(&daemon, Some("s1")).await;
        assert_eq!((shown.as_str(), report), ("", Some(Report::Gone)));
        let seen = daemon.seen.lock().unwrap().clone();
        assert_eq!(
            seen,
            ["GET /v0/chats/c1/terminals"],
            "no session made or killed"
        );
    }
}
