//! Ending the agent running on a chat, and listing the ones that run.

use sidebar::{RowId, RunningAgent};

use crate::after::After;
use crate::call::Call;
use crate::chat_record::ChatRecord;
use crate::host_slot::HostSlot;
use crate::plan::Plan;
use crate::plan_error::PlanError;
use crate::state::State;

const UNTITLED: &str = "Untitled chat";

pub(crate) fn kill(
    state: &State,
    row: &RowId,
) -> Result<Plan, PlanError> {
    let chat = row.as_str();
    let slot = state
        .hosts
        .iter()
        .find(|slot| slot.store.chats.get(chat).is_some())
        .ok_or(PlanError::ChatNotLoaded)?;
    let call = Call::post(format!("/v0/chats/{chat}/kill"));
    Ok(Plan::of(&slot.spec.id, vec![call]).then(After::ReloadChanges))
}

pub(crate) fn resume(
    state: &State,
    chat: &str,
) -> Result<Plan, PlanError> {
    let slot = state
        .hosts
        .iter()
        .find(|slot| slot.store.chats.get(chat).is_some())
        .ok_or(PlanError::ChatNotLoaded)?;
    let call = Call::post(format!("/v0/chats/{chat}/resume"));
    Ok(Plan::of(&slot.spec.id, vec![call]))
}

fn agent_of(
    slot: &HostSlot,
    chat: &ChatRecord,
) -> RunningAgent {
    let title = Some(chat.title.as_str())
        .filter(|title| !title.is_empty())
        .unwrap_or(UNTITLED);
    RunningAgent {
        row: RowId::from(chat.id.as_str()),
        title: title.to_owned(),
        viewers: slot
            .store
            .presence
            .get(&chat.id)
            .map_or(0, |seen| seen.viewers),
    }
}

pub(crate) fn running(state: &State) -> Vec<RunningAgent> {
    state
        .hosts
        .iter()
        .flat_map(|slot| {
            slot.store
                .chats
                .values()
                .filter(|chat| !chat.live_runner_id.is_empty())
                .map(|chat| agent_of(slot, chat))
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::state;

    #[test]
    fn a_chat_with_a_runner_is_listed_and_killed_through_its_route() {
        let (mut state, _) = state();
        let before = running(&state).len();
        let mut chat = state.hosts[0].store.chats.get("hc").unwrap().clone();
        chat.live_runner_id = "runner".into();
        state.hosts[0].store.chats.put("hc", 99, chat);
        let agents = running(&state);
        assert_eq!(agents.len(), before + 1);
        assert!(
            agents
                .iter()
                .any(|agent| agent.row.as_str() == "hc" && agent.viewers == 0)
        );
        let plan = kill(&state, &RowId::from("hc")).unwrap();
        assert_eq!(plan.after, After::ReloadChanges);
        let resumed = resume(&state, "hc").unwrap();
        assert_eq!(resumed.calls[0].path, "/v0/chats/hc/resume");
        assert!(resume(&state, "gone").is_err());
        assert_eq!(plan.calls[0].path, "/v0/chats/hc/kill");
        assert!(kill(&state, &RowId::from("gone")).is_err());
    }
}
