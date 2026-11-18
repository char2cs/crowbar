//! The first-run system check: one probe per tool, each answering on its own.

mod tool;
mod tool_report;

pub use tool::Tool;
pub use tool_report::ToolReport;

use std::collections::BTreeMap;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex};

use daemonapi::read::read;
use tokio::runtime::Handle;

use crate::live::ChangeSignal;

/// Asks this computer's daemon whether git and the forge CLIs are there. A probe that fails reads
/// as a tool that is not installed, so the screen can still guide the user.
pub struct SystemCheck {
    client: daemonapi::Client,
    handle: Handle,
    signal: ChangeSignal,
    round: AtomicU64,
    reports: Mutex<BTreeMap<Tool, ToolReport>>,
}

impl std::fmt::Debug for SystemCheck {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("SystemCheck")
            .finish_non_exhaustive()
    }
}

impl SystemCheck {
    pub(crate) fn start(
        client: daemonapi::Client,
        handle: Handle,
        signal: ChangeSignal,
    ) -> Arc<Self> {
        let check = Arc::new(Self {
            client,
            handle,
            signal,
            round: AtomicU64::new(0),
            reports: Mutex::default(),
        });
        check.probe_all(0);
        check
    }

    /// What the probe of `tool` found; `None` while it is still running.
    pub fn report(
        &self,
        tool: Tool,
    ) -> Option<ToolReport> {
        self.reports.lock().ok()?.get(&tool).cloned()
    }

    /// Forgets every answer and asks again.
    pub fn rerun(self: &Arc<Self>) {
        let round = self.round.fetch_add(1, Ordering::SeqCst) + 1;
        if let Ok(mut reports) = self.reports.lock() {
            reports.clear();
        }
        (self.signal)();
        self.probe_all(round);
    }

    fn probe_all(
        self: &Arc<Self>,
        round: u64,
    ) {
        for tool in Tool::ALL {
            self.spawn_probe(tool, round);
        }
    }

    fn spawn_probe(
        self: &Arc<Self>,
        tool: Tool,
        round: u64,
    ) {
        let check = Arc::clone(self);
        self.handle
            .spawn(async move { check.probe(tool, round).await });
    }

    async fn probe(
        &self,
        tool: Tool,
        round: u64,
    ) {
        let report = read::<ToolReport>(&self.client, tool.route().to_owned())
            .await
            .inspect_err(|error| tracing::warn!(?tool, %error, "a prerequisite probe failed"))
            .unwrap_or_default();
        if self.round.load(Ordering::SeqCst) != round {
            return;
        }
        if let Ok(mut reports) = self.reports.lock() {
            reports.insert(tool, report);
        }
        (self.signal)();
    }
}

#[cfg(test)]
mod tests {
    use tokio::sync::Notify;

    use super::*;
    use crate::test_support::state;

    async fn answered(
        check: &SystemCheck,
        changed: &Notify,
    ) {
        while Tool::ALL.iter().any(|tool| check.report(*tool).is_none()) {
            changed.notified().await;
        }
    }

    #[tokio::test]
    async fn each_tool_reports_alone_and_a_failed_probe_reads_as_absent() {
        let (state, daemon) = state();
        daemon.ok(
            "GET",
            "/v0/system/prerequisites/git",
            r#"{"installed":true,"version":"git version 2.45"}"#,
        );
        daemon.ok(
            "GET",
            "/v0/system/prerequisites/gh",
            r#"{"installed":true,"authed":false}"#,
        );
        daemon.answer(
            ("GET", "/v0/system/prerequisites/glab"),
            500,
            r#"{"success":false,"error":"boom"}"#,
        );
        let changed = Arc::new(Notify::new());
        let wake = Arc::clone(&changed);
        let signal: ChangeSignal = Arc::new(move || wake.notify_one());
        let client = state.hosts[0].client.clone();
        let check = SystemCheck::start(client, Handle::current(), signal);
        answered(&check, &changed).await;
        let git = check.report(Tool::Git).unwrap();
        assert!(git.installed && git.version.is_some());
        let gh = check.report(Tool::Gh).unwrap();
        assert!(gh.installed && !gh.authed);
        assert_eq!(check.report(Tool::Glab), Some(ToolReport::default()));
        check.rerun();
        answered(&check, &changed).await;
        assert!(check.report(Tool::Git).is_some(), "asked again");
    }
}
