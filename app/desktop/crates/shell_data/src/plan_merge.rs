//! The daemon requests of the merge box: strategy, merge, stop and merge, commit.

use serde_json::json;
use sidebar::{AfterMerge, MergeStrategy};

use crate::call::Call;
use crate::files_reads::followed_workspace;
use crate::merge_reads::strategy_wire;
use crate::new_id;
use crate::open_chat::OpenChat;
use crate::plan::Plan;
use crate::state::State;

pub(crate) fn set_strategy(
    state: &State,
    strategy: MergeStrategy,
) -> Result<Plan, String> {
    let held = OpenChat::of(state)?;
    let call = Call::patch(format!("/v0/chats/{}/review", held.chat))
        .json(json!({"mergeStrategy": strategy_wire(strategy)}));
    Ok(Plan::of(held.host, vec![call]))
}

fn merge_call(
    state: &State,
    chat: &str,
    after: AfterMerge,
) -> Result<Call, String> {
    let workspace = followed_workspace(state).ok_or("this workspace is not loaded yet")?;
    let strategy = match workspace.merge_strategy.as_str() {
        "squash" | "rebase" => workspace.merge_strategy.as_str(),
        _ => "merge",
    };
    let delete = after == AfterMerge::DeleteBranch;
    Ok(Call::post(format!("/v0/chats/{chat}/merge-into-parent"))
        .json(json!({"strategy": strategy, "deleteSource": delete}))
        .idempotent(&new_id::uuid()))
}

pub(crate) fn merge(
    state: &State,
    after: AfterMerge,
) -> Result<Plan, String> {
    let held = OpenChat::of(state)?;
    let call = merge_call(state, held.chat, after)?;
    Ok(Plan::of(held.host, vec![call]))
}

pub(crate) fn stop_and_merge(state: &State) -> Result<Plan, String> {
    let held = OpenChat::of(state)?;
    let workspace = followed_workspace(state).ok_or("this workspace is not loaded yet")?;
    let mut calls: Vec<Call> = state
        .hosts
        .iter()
        .flat_map(|slot| slot.store.chats.values())
        .filter(|chat| chat.workspace_id == workspace.id && !chat.live_runner_id.is_empty())
        .map(|chat| Call::post(format!("/v0/chats/{}/stop", chat.id)))
        .collect();
    calls.push(merge_call(state, held.chat, AfterMerge::DeleteBranch)?);
    Ok(Plan::of(held.host, calls))
}

pub(crate) fn commit(
    state: &State,
    (message, paths): (&str, &[String]),
) -> Result<Plan, String> {
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
    Ok(Plan::of(held.host, calls))
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
    }

    #[test]
    fn without_an_open_workspace_every_merge_command_is_refused() {
        let (state, _) = state();
        assert!(merge(&state, AfterMerge::Keep).is_err());
        assert!(stop_and_merge(&state).is_err());
        assert!(set_strategy(&state, MergeStrategy::Merge).is_err());
    }
}
