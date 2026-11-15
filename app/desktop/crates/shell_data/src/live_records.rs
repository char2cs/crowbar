//! The Active sessions and Hosts lists, read from what the streams hold.

use settings_ui::{Line, Record, RecordAction, Tone};

use crate::chat_record::ChatRecord;
use crate::host_slot::HostSlot;
use crate::link_state::LinkState;
use crate::row_look::title_of;
use crate::state::State;
use crate::switch::Switch;

pub(crate) const KILL: &str = "kill";
const THIS_COMPUTER: &str = "This computer";

pub(crate) fn sessions(state: &State) -> Vec<Record> {
    let mut chats: Vec<(&HostSlot, &ChatRecord)> = state
        .hosts
        .iter()
        .flat_map(|slot| slot.store.chats.values().map(move |chat| (slot, chat)))
        .filter(|(_, chat)| !chat.live_runner_id.is_empty())
        .collect();
    chats.sort_by(|(_, a), (_, b)| {
        a.created_at
            .cmp(&b.created_at)
            .then_with(|| a.id.cmp(&b.id))
    });
    chats
        .into_iter()
        .map(|(slot, chat)| session(slot, chat))
        .collect()
}

fn session(
    slot: &HostSlot,
    chat: &ChatRecord,
) -> Record {
    Record::new(chat.id.as_str(), title_of(&chat.title))
        .status(session_tone(chat))
        .line(Line::plain(slot.spec.name.as_str()))
        .action(RecordAction::new(KILL, "x", "End this session"))
}

fn session_tone(chat: &ChatRecord) -> Tone {
    match chat.working {
        Switch::On => Tone::Pending,
        Switch::Off => Tone::Success,
    }
}

pub(crate) fn hosts(state: &State) -> Vec<Record> {
    state.hosts.iter().map(host).collect()
}

fn host(slot: &HostSlot) -> Record {
    let address = slot.spec.endpoint.as_deref().unwrap_or(THIS_COMPUTER);
    Record::new(slot.spec.id.as_str(), slot.spec.name.as_str())
        .status(link_tone(slot.link))
        .line(Line::plain(address).mono())
}

fn link_tone(link: LinkState) -> Tone {
    match link {
        LinkState::Live => Tone::Success,
        LinkState::Connecting => Tone::Pending,
        LinkState::Down => Tone::Error,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::state;

    #[test]
    fn only_chats_with_a_running_agent_are_sessions_and_each_host_is_listed() {
        let (state, _) = state();
        let sessions = sessions(&state);
        assert_eq!(sessions.iter().map(Record::id).collect::<Vec<_>>(), ["bo"]);
        assert_eq!(
            hosts(&state).iter().map(Record::id).collect::<Vec<_>>(),
            ["local"]
        );
    }
}
