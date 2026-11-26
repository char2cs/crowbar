//! Waiting for an operation a call started, as the host stream reports it.
//!
//! Spec: develop `web/src/core/entities/run-operation.ts` (`whenSettled`): no timeout, each
//! outcome is a frame the daemon sends.

use daemonapi::Call;

use crate::live::Hub;
use crate::live::OperationRecord;
use crate::live::PlanError;
use crate::live::State;

const KEY: &str = "idempotency-key";
const SUCCEEDED: &str = "succeeded";
const FAILED: &str = "failed";

pub(crate) fn starts(
    call: &Call,
    operation: &str,
) -> bool {
    call.headers
        .iter()
        .any(|(name, value)| name == KEY && value == operation)
}

pub(crate) async fn settled(
    hub: &Hub,
    (host, operation): (&str, Option<String>),
) -> Result<(), PlanError> {
    let Some(operation) = operation else {
        return Ok(());
    };
    let gone = || Err(PlanError::UnknownHost(host.to_owned()));
    hub.until(|state| outcome_of(state, (host, &operation)))
        .await
        .unwrap_or_else(gone)
}

fn outcome_of(
    state: &State,
    (host, operation): (&str, &str),
) -> Option<Result<(), PlanError>> {
    let Some(slot) = state.hosts.iter().find(|slot| slot.spec.id == host) else {
        return Some(Err(PlanError::UnknownHost(host.to_owned())));
    };
    let op = slot.store.operations.get(operation)?;
    match op.state.as_str() {
        SUCCEEDED => Some(Ok(())),
        FAILED => Some(Err(PlanError::Operation(failure_of(op)))),
        _ => None,
    }
}

fn failure_of(op: &OperationRecord) -> String {
    let items = op.items.iter().filter_map(|item| item.error.as_ref());
    op.error
        .iter()
        .chain(items)
        .map(|problem| problem.message.clone())
        .next()
        .unwrap_or_else(|| format!("{} failed", op.label))
}

#[cfg(test)]
mod tests {
    use std::sync::{Arc, Mutex};

    use daemonapi::Frame;
    use serde_json::json;

    use super::*;
    use crate::live::{Outcome, OutcomeSink};
    use crate::plan::Plan;
    use crate::plan::runner::run;
    use crate::test_support::{FakeDaemon, state};

    const REPARENT: &str = "/v0/reparent";
    const PLACE: &str = "/v0/placement";

    type Heard = Arc<Mutex<Vec<Outcome>>>;

    fn sink() -> (OutcomeSink, Heard) {
        let seen = Arc::new(Mutex::new(Vec::new()));
        let keep = Arc::clone(&seen);
        let sink: OutcomeSink = Arc::new(move |outcome| keep.lock().unwrap().push(outcome));
        (sink, seen)
    }

    fn operation(state: &str) -> Frame {
        let problem = json!({"code": "conflict", "message": "the rebase stopped on a conflict"});
        Frame::Entity {
            scope: "projects".into(),
            kind: "operation".into(),
            id: "op".into(),
            v: 1,
            data: Some(json!({"id": "op", "state": state, "label": "Move feat", "error": problem})),
            gone: false,
        }
    }

    fn moving() -> (Arc<Hub>, Arc<FakeDaemon>, Plan) {
        let (state, daemon) = state();
        daemon.ok("POST", REPARENT, r#"{"operation":{"id":"op"}}"#);
        daemon.ok("PATCH", PLACE, "{}");
        let hub = Hub::new(state, Arc::new(|| {}));
        let calls = vec![Call::post(REPARENT).idempotent("op"), Call::patch(PLACE)];
        let plan = Plan::of("local", calls).settling("op".into());
        (hub, daemon, plan)
    }

    fn spawned(
        hub: &Arc<Hub>,
        plan: Plan,
        outcome: OutcomeSink,
    ) -> tokio::task::JoinHandle<()> {
        let hub = Arc::clone(hub);
        tokio::spawn(async move { run(&hub, plan, &outcome).await })
    }

    fn paths(daemon: &FakeDaemon) -> Vec<String> {
        daemon.seen().into_iter().map(|seen| seen.path).collect()
    }

    async fn the_next_call_waits_until_the_stream_says_the_operation_succeeded() {
        let (hub, daemon, plan) = moving();
        hub.edit(|state| state.apply("local", operation("running")));
        let (outcome, _) = sink();
        let running = spawned(&hub, plan, outcome);
        tokio::task::yield_now().await;
        assert_eq!(paths(&daemon), [REPARENT], "waiting");
        hub.edit(|state| state.apply("local", operation("succeeded")));
        running.await.unwrap();
        assert_eq!(paths(&daemon), [REPARENT, PLACE], "settled");
    }

    async fn a_failed_operation_stops_the_plan_with_its_reason() {
        let (hub, daemon, plan) = moving();
        hub.edit(|state| state.apply("local", operation("failed")));
        let (outcome, heard) = sink();
        run(&hub, plan, &outcome).await;
        assert_eq!(paths(&daemon), [REPARENT]);
        let reason = "the rebase stopped on a conflict".to_owned();
        assert_eq!(*heard.lock().unwrap(), [Outcome::Failed(reason)]);
    }

    async fn the_wait_ends_when_the_host_is_removed() {
        let (hub, daemon, plan) = moving();
        let (outcome, heard) = sink();
        let running = spawned(&hub, plan, outcome);
        tokio::task::yield_now().await;
        hub.edit(|state| state.remove_host("local"));
        running.await.unwrap();
        assert_eq!(paths(&daemon), [REPARENT]);
        let gone = Outcome::Failed("host local is not known".into());
        assert_eq!(*heard.lock().unwrap(), [gone]);
    }

    async fn a_dropped_plan_lets_go_of_the_store() {
        let (hub, _, plan) = moving();
        let (outcome, _) = sink();
        let running = spawned(&hub, plan, outcome);
        tokio::task::yield_now().await;
        running.abort();
        assert!(running.await.unwrap_err().is_cancelled());
        assert_eq!(Arc::strong_count(&hub), 1);
    }

    type Case = fn() -> std::pin::Pin<Box<dyn Future<Output = ()>>>;

    const CASES: [Case; 4] = [
        || Box::pin(the_next_call_waits_until_the_stream_says_the_operation_succeeded()),
        || Box::pin(a_failed_operation_stops_the_plan_with_its_reason()),
        || Box::pin(the_wait_ends_when_the_host_is_removed()),
        || Box::pin(a_dropped_plan_lets_go_of_the_store()),
    ];

    async fn run_all() {
        for case in CASES {
            case().await;
        }
    }

    #[tokio::test]
    async fn a_plan_waits_for_the_operation_it_started_and_no_longer() {
        run_all().await;
    }
}
