//! A remote host that did not answer for its agent list, before anything was created: told in
//! its host's words, with no operation row, as develop's `readUnreadHostProviders` does.
//!
//! Spec: develop `web/src/components/layout/thread-create.ts` (`readUnreadHostProviders`).

use daemonapi::HostNotice;

use crate::error::Error;
use crate::live::{Hub, Outcome, OutcomeSink, PlanError, State};
use crate::plan::Plan;

pub(crate) fn unanswered(
    state: &State,
    host: &str,
    error: Error,
) -> PlanError {
    let remote = state
        .hosts
        .iter()
        .find(|slot| slot.spec.id == host && slot.spec.endpoint.is_some());
    let Some(slot) = remote else {
        return PlanError::Daemon(error);
    };
    let name = slot.spec.name.as_str();
    let named = match &error {
        Error::Daemon(daemon) => HostNotice::of(daemon, Some(name)),
        _ => None,
    };
    PlanError::HostUnanswered(named.unwrap_or_else(|| HostNotice::unreachable(name)))
}

pub(crate) fn tell(
    hub: &Hub,
    plan: &Plan,
    outcome: &OutcomeSink,
    notice: HostNotice,
) {
    if let Some(row) = &plan.pending {
        hub.edit(|state| state.drop_local(&row.id));
    }
    let Some(reason) = notice.description else {
        return outcome(Outcome::Failed(notice.message));
    };
    outcome(Outcome::Refused {
        title: notice.message,
        reason,
    });
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use daemonapi::Call;

    use super::*;
    use crate::live::{HostSpec, PendingRow};
    use crate::plan::runner::run;
    use crate::test_support::{FakeDaemon, recorder, state, state_on};

    const PROVIDERS: &str = "/v0/settings/chat/providers";
    const CREATE: &str = "/v0/projects/p/home/chats";

    fn devbox() -> (Arc<Hub>, Arc<FakeDaemon>) {
        let (state, daemon) = state_on(HostSpec {
            id: "devbox".into(),
            name: "devbox".into(),
            endpoint: Some("10.0.0.2:4747".into()),
        });
        (Hub::new(state, Arc::new(|| {})), daemon)
    }

    fn creating(host: &str) -> Plan {
        let row = PendingRow {
            id: "n".into(),
            label: "New thread".into(),
            ..PendingRow::default()
        };
        Plan::of(host, vec![Call::post(CREATE)])
            .showing(row)
            .with_provider()
            .failing_as(Some("New thread failed".into()))
    }

    async fn told(
        hub: &Hub,
        plan: Plan,
    ) -> Vec<Outcome> {
        let (sink, heard) = recorder();
        run(hub, plan, &sink).await;
        heard.lock().unwrap().clone()
    }

    #[tokio::test]
    async fn a_remote_host_that_does_not_answer_for_its_agents_is_told_by_name_with_no_row() {
        let (hub, daemon) = devbox();
        daemon.proxied(("GET", PROVIDERS), 503, protocol::HOST_UNREACHABLE);
        let unreachable = Outcome::Refused {
            title: "Couldn\u{2019}t reach devbox".into(),
            reason: "Check that it is online in Settings \u{2192} Hosts.".into(),
        };
        assert_eq!(told(&hub, creating("devbox")).await, [unreachable]);
        assert!(
            hub.read(|state| state.local_rows.is_empty()),
            "no row is left"
        );
        daemon.answer(
            ("GET", PROVIDERS),
            401,
            r#"{"success":false,"error":"client is not paired"}"#,
        );
        let refused = told(&hub, creating("devbox")).await;
        assert!(
            matches!(&refused[..], [Outcome::Refused { title, .. }] if title == "devbox needs to be paired again"),
            "{refused:?}"
        );
        assert!(
            daemon.seen().iter().all(|call| call.method == "GET"),
            "nothing was created"
        );
    }

    #[tokio::test]
    async fn a_failed_create_keeps_its_title_and_reads_the_transports_words() {
        let (hub, daemon) = devbox();
        daemon.ok(
            "GET",
            PROVIDERS,
            r#"[{"id":"claude","enabled":true,"connected":true}]"#,
        );
        daemon.proxied(("POST", CREATE), 502, protocol::HOST_IDENTITY_CHANGED);
        let identity = Outcome::Refused {
            title: "New thread failed".into(),
            reason: "this host's identity changed \u{2014} its certificate no longer matches what was pinned".into(),
        };
        assert_eq!(told(&hub, creating("devbox")).await, [identity]);
        let (state, local) = state();
        let hub = Hub::new(state, Arc::new(|| ()));
        local.answer(
            ("GET", PROVIDERS),
            500,
            r#"{"success":false,"error":"settings unreadable"}"#,
        );
        let own = Outcome::Refused {
            title: "New thread failed".into(),
            reason: "settings unreadable".into(),
        };
        assert_eq!(
            told(&hub, creating("local")).await,
            [own],
            "this computer keeps its own words"
        );
    }
}
