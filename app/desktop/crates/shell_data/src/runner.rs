//! Carrying out a plan: the requests in order, the local row meanwhile, the outcome after.

use crate::after::After;
use crate::call::Call;
use crate::changes_reload;
use crate::daemon_client::DaemonClient;
use crate::hub::Hub;
use crate::outcome::{Outcome, OutcomeSink};
use crate::plan::Plan;
use crate::plan_error::PlanError;
use crate::provider::Provider;
use crate::providers;
use crate::state::State;
use crate::surface::Surface;

pub(crate) async fn run(
    hub: &Hub,
    plan: Plan,
    outcome: &OutcomeSink,
) {
    if let Some(id) = plan.dropping.as_deref() {
        hub.edit(|state| state.drop_local(id));
        return;
    }
    let row_id = plan.pending.as_ref().map(|row| row.id.clone());
    if let Some(row) = plan.pending.clone() {
        hub.edit(|state| state.add_local(row));
    }
    if plan.committing {
        hub.edit(|state| state.panels.committing = true);
    }
    match carry_out(hub, &plan).await {
        Ok(()) => finish(hub, plan.after, outcome).await,
        Err(reason) => fail(hub, row_id.as_deref(), outcome, &reason.to_string()),
    }
    if plan.committing {
        hub.edit(|state| state.panels.committing = false);
    }
}

async fn carry_out(
    hub: &Hub,
    plan: &Plan,
) -> Result<(), PlanError> {
    let client = hub
        .read(|state| client_of(state, &plan.host))
        .ok_or_else(|| PlanError::UnknownHost(plan.host.clone()))?;
    let provider = chosen_provider(hub, &client, plan).await?;
    for call in plan.calls.iter().cloned() {
        send(&client, call, provider.as_ref().zip(Some(plan.landing))).await?;
    }
    Ok(())
}

async fn chosen_provider(
    hub: &Hub,
    client: &DaemonClient,
    plan: &Plan,
) -> Result<Option<Provider>, PlanError> {
    if !plan.needs_provider {
        return Ok(None);
    }
    pick_provider(client, hub, &plan.host).await.map(Some)
}

async fn send(
    client: &DaemonClient,
    mut call: Call,
    chosen: Option<(&Provider, Surface)>,
) -> Result<(), PlanError> {
    if let Some((body, (provider, landing))) = call.body.as_mut().zip(chosen) {
        providers::fill(body, &provider.id);
        providers::land(body, (provider, landing));
    }
    client.send(call).await.map(drop).map_err(PlanError::Daemon)
}

fn client_of(
    state: &State,
    host: &str,
) -> Option<DaemonClient> {
    state
        .hosts
        .iter()
        .find(|slot| slot.spec.id == host)
        .map(|slot| slot.client.clone())
}

async fn pick_provider(
    client: &DaemonClient,
    hub: &Hub,
    host: &str,
) -> Result<Provider, PlanError> {
    let all = providers::list(client).await.map_err(PlanError::Daemon)?;
    let terminal = all.iter().any(Provider::offers_terminal);
    hub.edit(|state| state.set_terminal_threads(host, Some(terminal).into()));
    providers::pick(&all)
        .cloned()
        .ok_or(PlanError::NoAgentEnabled)
}

async fn finish(
    hub: &Hub,
    after: After,
    outcome: &OutcomeSink,
) {
    match after {
        After::Nothing => {},
        After::OpenChat(id) => outcome(Outcome::OpenChat(id)),
        After::SelectProject(id) => hub.edit(|state| state.active_project = Some(id)),
        After::ReloadChanges => changes_reload::reload(hub).await,
    }
}

fn fail(
    hub: &Hub,
    row: Option<&str>,
    outcome: &OutcomeSink,
    reason: &str,
) {
    if let Some(id) = row {
        hub.edit(|state| state.fail_local(id, reason));
    }
    outcome(Outcome::Failed(reason.to_owned()));
}

#[cfg(test)]
mod tests {
    use std::sync::{Arc, Mutex};

    use serde_json::json;

    use super::*;
    use crate::after::After;
    use crate::call::Call;
    use crate::pending_row::PendingRow;
    use crate::test_support::state;

    type Heard = Arc<Mutex<Vec<Outcome>>>;

    fn sink() -> (OutcomeSink, Heard) {
        let seen = Arc::new(Mutex::new(Vec::new()));
        let keep = Arc::clone(&seen);
        let sink: OutcomeSink = Arc::new(move |outcome| keep.lock().unwrap().push(outcome));
        (sink, seen)
    }

    fn pending(id: &str) -> PendingRow {
        PendingRow {
            id: id.into(),
            label: "New thread".into(),
            scope: String::new(),
            parent: String::new(),
            failure: None,
            operation: None,
        }
    }

    async fn a_create_picks_an_agent_sends_its_call_and_opens_the_chat() {
        let (state, daemon) = state();
        daemon.ok("GET", "/v0/settings/chat/providers", r#"[{"id":"codex","enabled":false},{"id":"claude","enabled":true,"connected":true,"hasTerminal":true}]"#);
        daemon.ok("POST", "/v0/projects/p/home/chats", r#"{"id":"n"}"#);
        let hub = Hub::new(state, Arc::new(|| {}));
        let plan = Plan::of(
            "local",
            vec![
                Call::post("/v0/projects/p/home/chats").json(json!({"id": "n", "provider": null})),
            ],
        )
        .showing(pending("n"))
        .then(After::OpenChat("n".into()))
        .with_provider();
        let (outcome, seen) = sink();
        run(&hub, plan, &outcome).await;
        let posted = daemon
            .seen()
            .into_iter()
            .find(|call| call.method == "POST")
            .unwrap();
        assert_eq!(posted.body.unwrap()["provider"], "claude");
        assert_eq!(*seen.lock().unwrap(), [Outcome::OpenChat("n".into())]);
        assert!(hub.read(|state| state.hosts[0].terminal_threads));
        assert_eq!(hub.read(|state| state.local_rows.len()), 1);
    }

    async fn surface_posted(able: &str) -> Option<String> {
        let (state, daemon) = state();
        let listed = format!(
            r#"[{{"id":"claude","enabled":true,"connected":true,"hasTerminal":true,"terminalStartHere":{able}}}]"#
        );
        daemon.ok("GET", "/v0/settings/chat/providers", &listed);
        daemon.ok("POST", "/v0/projects/p/home/chats", r#"{"id":"n"}"#);
        let hub = Hub::new(state, Arc::new(|| {}));
        let body = json!({"id": "n", "provider": null});
        let call = Call::post("/v0/projects/p/home/chats").json(body);
        let plan = Plan::of("local", vec![call])
            .with_provider()
            .landing_on(Surface::Terminal);
        run(&hub, plan, &sink().0).await;
        let posted = daemon
            .seen()
            .into_iter()
            .find(|call| call.method == "POST")?;
        posted.body?["surface"].as_str().map(str::to_owned)
    }

    async fn a_terminal_landing_reaches_the_create_only_when_the_chosen_agent_can_start_there() {
        assert_eq!(surface_posted("true").await.as_deref(), Some("terminal"));
        assert_eq!(surface_posted("false").await, None);
    }

    async fn a_refusal_stays_on_the_pending_row_and_reaches_the_shell() {
        let (state, daemon) = state();
        daemon.answer(
            ("POST", "/v0/projects/p/home/chats"),
            409,
            r#"{"success":false,"error":"workspace is busy"}"#,
        );
        let hub = Hub::new(state, Arc::new(|| {}));
        let plan =
            Plan::of("local", vec![Call::post("/v0/projects/p/home/chats")]).showing(pending("n"));
        let (outcome, seen) = sink();
        run(&hub, plan, &outcome).await;
        assert_eq!(
            hub.read(|state| state.local_rows[0].failure.clone())
                .as_deref(),
            Some("workspace is busy")
        );
        assert_eq!(
            *seen.lock().unwrap(),
            [Outcome::Failed("workspace is busy".into())]
        );
    }

    async fn no_enabled_agent_stops_before_any_create_and_a_drop_plan_only_forgets() {
        let (state, daemon) = state();
        daemon.ok("GET", "/v0/settings/chat/providers", "[]");
        let hub = Hub::new(state, Arc::new(|| {}));
        let (outcome, seen) = sink();
        let plan = Plan::of("local", vec![Call::post("/v0/projects/p/home/chats")]).with_provider();
        run(&hub, plan, &outcome).await;
        assert!(daemon.seen().iter().all(|call| call.method == "GET"));
        assert!(matches!(seen.lock().unwrap()[0], Outcome::Failed(_)));
        hub.edit(|state| state.add_local(pending("x")));
        let mut forget = Plan::of("", Vec::new());
        forget.dropping = Some("x".into());
        run(&hub, forget, &outcome).await;
        assert!(hub.read(|state| state.local_rows.is_empty()));
    }

    async fn selecting_a_created_space_makes_it_active() {
        let (state, daemon) = state();
        daemon.ok("POST", "/v0/projects", "{}");
        let hub = Hub::new(state, Arc::new(|| {}));
        let plan = Plan::of("local", vec![Call::post("/v0/projects")])
            .then(After::SelectProject("q".into()));
        run(&hub, plan, &sink().0).await;
        assert_eq!(
            hub.read(|state| state.active_project.clone()).as_deref(),
            Some("q")
        );
    }

    async fn a_commit_is_marked_while_it_runs_and_unmarked_when_it_ends_either_way() {
        let (state, daemon) = state();
        daemon.ok("POST", "/v0/chats/hc/git/commit", "{}");
        daemon.answer(
            ("POST", "/v0/chats/hc/git/refused"),
            409,
            r#"{"success":false,"error":"nothing to commit"}"#,
        );
        let hub = Hub::new(state, Arc::new(|| {}));
        for path in ["commit", "refused"] {
            let call = Call::post(format!("/v0/chats/hc/git/{path}"));
            run(&hub, Plan::of("local", vec![call]).committing(), &sink().0).await;
            assert!(!hub.read(|state| state.panels.committing), "{path}");
        }
    }

    type Case = fn() -> std::pin::Pin<Box<dyn Future<Output = ()>>>;

    const CASES: [Case; 6] = [
        || Box::pin(a_create_picks_an_agent_sends_its_call_and_opens_the_chat()),
        || {
            Box::pin(
                a_terminal_landing_reaches_the_create_only_when_the_chosen_agent_can_start_there(),
            )
        },
        || Box::pin(a_refusal_stays_on_the_pending_row_and_reaches_the_shell()),
        || Box::pin(no_enabled_agent_stops_before_any_create_and_a_drop_plan_only_forgets()),
        || Box::pin(selecting_a_created_space_makes_it_active()),
        || Box::pin(a_commit_is_marked_while_it_runs_and_unmarked_when_it_ends_either_way()),
    ];

    async fn run_all() {
        for case in CASES {
            case().await;
        }
    }

    #[tokio::test]
    async fn every_run_ends_as_its_command_says() {
        run_all().await;
    }
}
