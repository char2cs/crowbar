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
    use daemonapi::Call;

    use super::*;
    use crate::plan::runner::run;
    use crate::test_support::{recorder, state, watched};

    const FOLDER: &str = "/w/app";

    #[tokio::test]
    async fn a_folder_is_adding_while_its_request_runs_and_announced_only_once_added() {
        let (state, daemon) = state();
        daemon.ok("POST", "/v0/projects", "{}");
        daemon.answer(("POST", "/v0/refused"), 500, r#"{"error":"no"}"#);
        let (hub, seen) = watched(state, |state| state.adding.contains(FOLDER));
        let (sink, outcomes) = recorder();
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
