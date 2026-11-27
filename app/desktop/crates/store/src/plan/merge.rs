mod open_chat;

use daemonapi::Call;
use serde_json::json;
use sidebar::{AfterMerge, MergeStrategy};

use crate::live::PlanError;
use crate::live::State;
use crate::plan::Plan;
use crate::plan::after::After;
use crate::plan::merge::open_chat::OpenChat;
use crate::reads::followed_workspace;
use crate::reads::strategy_wire;

pub(crate) fn set_strategy(
    state: &State,
    strategy: MergeStrategy,
) -> Result<Plan, PlanError> {
    let held = OpenChat::of(state)?;
    let call = Call::patch(format!("/v0/chats/{}/review", held.chat))
        .json(json!({"mergeStrategy": strategy_wire(strategy)}));
    Ok(Plan::of(held.host, vec![call]))
}

fn merge_call(
    state: &State,
    chat: &str,
    after: AfterMerge,
) -> Result<Call, PlanError> {
    let workspace = followed_workspace(state).ok_or(PlanError::WorkspaceNotLoaded)?;
    let strategy = match workspace.merge_strategy.as_str() {
        "squash" | "rebase" => workspace.merge_strategy.as_str(),
        _ => "merge",
    };
    let delete = after == AfterMerge::DeleteBranch;
    Ok(Call::post(format!("/v0/chats/{chat}/merge-into-parent"))
        .json(json!({"strategy": strategy, "deleteSource": delete}))
        .idempotent(&daemonapi::Uuid::random().to_string()))
}

pub(crate) fn merge(
    state: &State,
    after: AfterMerge,
) -> Result<Plan, PlanError> {
    let held = OpenChat::of(state)?;
    let call = merge_call(state, held.chat, after)?;
    Ok(Plan::of(held.host, vec![call]).then(After::ReloadChanges))
}

pub(crate) fn stop_and_merge(state: &State) -> Result<Plan, PlanError> {
    let held = OpenChat::of(state)?;
    let workspace = followed_workspace(state).ok_or(PlanError::WorkspaceNotLoaded)?;
    let mut calls: Vec<Call> = state
        .hosts
        .iter()
        .flat_map(|slot| slot.store.chats.values())
        .filter(|chat| chat.workspace_id == workspace.id && !chat.live_runner_id.is_empty())
        .map(|chat| Call::post(format!("/v0/chats/{}/stop", chat.id)))
        .collect();
    calls.push(merge_call(state, held.chat, AfterMerge::DeleteBranch)?);
    Ok(Plan::of(held.host, calls).then(After::ReloadChanges))
}

pub(crate) fn commit(
    state: &State,
    (message, paths): (&str, &[String]),
) -> Result<Plan, PlanError> {
    let held = OpenChat::of(state)?;
    let base = format!("/v0/chats/{}/git", held.chat);
    let (subject, body) = message
        .trim()
        .split_once("\n\n")
        .unwrap_or((message.trim(), ""));
    let unstage: Vec<&String> = state
        .panels
        .staged
        .iter()
        .filter(|path| !paths.contains(path))
        .collect();
    let mut calls = vec![Call::post(format!("{base}/stage")).json(json!({"paths": paths}))];
    if !unstage.is_empty() {
        calls.push(Call::post(format!("{base}/unstage")).json(json!({"paths": unstage})));
    }
    calls.push(
        Call::post(format!("{base}/commit"))
            .json(json!({"subject": subject.trim(), "body": body.trim()})),
    );
    Ok(Plan::of(held.host, calls)
        .then(After::ReloadChanges)
        .committing())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::state;

    fn open() -> State {
        let (mut state, _) = state();
        state.panels.follow(Some("hc".into()));
        state
    }

    fn routes(plan: &Plan) -> Vec<(&'static str, &str)> {
        plan.calls
            .iter()
            .map(|call| (call.method, call.path.as_str()))
            .collect()
    }

    #[test]
    fn the_merge_box_commands_become_their_daemon_calls() {
        let state = open();
        let reloading = [
            merge(&state, AfterMerge::Keep),
            stop_and_merge(&state),
            commit(&state, ("m", &[])),
        ];
        assert!(
            reloading
                .iter()
                .all(|plan| plan.as_ref().unwrap().after == After::ReloadChanges),
            "the Changes panel is read again once the work changed"
        );
        let strategy = set_strategy(&state, MergeStrategy::Squash).unwrap();
        assert_eq!(routes(&strategy), [("PATCH", "/v0/chats/hc/review")]);
        assert_eq!(
            strategy.calls[0].body,
            Some(json!({"mergeStrategy": "squash"}))
        );
        let merged = merge(&state, AfterMerge::DeleteBranch).unwrap();
        assert_eq!(
            merged.calls[0].body,
            Some(json!({"strategy": "merge", "deleteSource": true}))
        );
        let paths = vec!["a.rs".to_owned()];
        let committed = commit(&state, ("Fix it\n\nbecause", &paths)).unwrap();
        assert_eq!(
            routes(&committed),
            [
                ("POST", "/v0/chats/hc/git/stage"),
                ("POST", "/v0/chats/hc/git/commit")
            ]
        );
        assert_eq!(
            committed.calls[1].body,
            Some(json!({"subject": "Fix it", "body": "because"}))
        );
        assert!(committed.committing && !merged.committing);
    }

    #[test]
    fn without_an_open_workspace_every_merge_command_is_refused() {
        let (state, _) = state();
        assert!(merge(&state, AfterMerge::Keep).is_err());
        assert!(stop_and_merge(&state).is_err());
        assert!(set_strategy(&state, MergeStrategy::Merge).is_err());
    }
}
