//! What the state holds while a plan runs: the commit, the icon and the folder being added.

use crate::live::State;
use crate::live::{Outcome, OutcomeSink};
use crate::plan::Plan;

pub(super) fn holds(plan: &Plan) -> bool {
    plan.committing || plan.marking.is_some() || plan.adding.is_some()
}

pub(super) fn hold(
    state: &mut State,
    plan: &Plan,
) {
    state.panels.committing |= plan.committing;
    state.marking.extend(plan.marking.clone());
    state.adding.extend(plan.adding.clone());
}

pub(super) fn release(
    state: &mut State,
    plan: &Plan,
) {
    if plan.committing {
        state.panels.committing = false;
    }
    if let Some(target) = &plan.marking {
        state.marking.remove(target);
    }
    if let Some(folder) = &plan.adding {
        state.adding.remove(folder);
    }
}

pub(super) fn announce(
    plan: &Plan,
    outcome: &OutcomeSink,
) {
    if let Some(folder) = &plan.adding {
        outcome(Outcome::Added(folder.clone()));
    }
}

#[cfg(test)]
mod tests {
    use std::sync::{Arc, Mutex, OnceLock, Weak};

    use daemonapi::Call;

    use super::*;
    use crate::live::{ChangeSignal, Hub};
    use crate::plan::runner::run;
    use crate::test_support::state;

    const FOLDER: &str = "/w/app";

    type Seen = Arc<Mutex<Vec<bool>>>;

    fn watched_hub(state: State) -> (Arc<Hub>, Seen) {
        let hub_of = Arc::new(OnceLock::<Weak<Hub>>::new());
        let seen = Arc::new(Mutex::new(Vec::new()));
        let (reader, heard) = (Arc::clone(&hub_of), Arc::clone(&seen));
        let signal: ChangeSignal = Arc::new(move || {
            let hub = reader.get().and_then(Weak::upgrade);
            let adding = hub.map(|hub| hub.read(|state| state.adding.contains(FOLDER)));
            heard.lock().unwrap().extend(adding);
        });
        let hub = Hub::new(state, signal);
        hub_of.set(Arc::downgrade(&hub)).unwrap();
        (hub, seen)
    }

    #[tokio::test]
    async fn a_folder_is_adding_while_its_request_runs_and_announced_only_once_added() {
        let (state, daemon) = state();
        daemon.ok("POST", "/v0/projects", "{}");
        daemon.answer(("POST", "/v0/refused"), 500, r#"{"error":"no"}"#);
        let (hub, seen) = watched_hub(state);
        let outcomes = Arc::new(Mutex::new(Vec::new()));
        let keep = Arc::clone(&outcomes);
        let sink: OutcomeSink = Arc::new(move |outcome| keep.lock().unwrap().push(outcome));
        let plan = |path: &str| Plan::of("local", vec![Call::post(path)]).adding(FOLDER);
        run(&hub, plan("/v0/projects"), &sink).await;
        run(&hub, plan("/v0/refused"), &sink).await;
        assert_eq!(*seen.lock().unwrap(), [true, false, true, false]);
        let added: Vec<_> = outcomes
            .lock()
            .unwrap()
            .iter()
            .filter(|outcome| matches!(outcome, Outcome::Added(_)))
            .cloned()
            .collect();
        assert_eq!(
            added,
            [Outcome::Added(FOLDER.into())],
            "the refused add stays open"
        );
    }
}
