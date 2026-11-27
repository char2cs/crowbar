use daemonapi::Switch;
use settings::{Confirm, Line, Record, RecordAction, Tone};
use sidebar::{HostState, HostTone};

use crate::live::ChatRecord;
use crate::live::HostSlot;
use crate::live::LinkState;
use crate::live::State;
use crate::rows::title_of;

pub(crate) const KILL: &str = "kill";
pub(crate) const REVOKE: &str = "revoke";
pub(crate) const REMOVE_ANYWAY: &str = "remove-anyway";
const THIS_COMPUTER: &str = "This computer";
const CLOUD: &str = "cloud";
const DESKTOP: &str = "desktop";

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
    let title = title_of(&chat.title);
    Record::new(chat.id.as_str(), title)
        .status(session_tone(chat))
        .line(Line::plain(slot.spec.name.as_str()))
        .action(kill(chat, title))
}

fn kill(
    chat: &ChatRecord,
    title: &str,
) -> RecordAction {
    let question = sidebar::Question::kill(sidebar::RowId::from(chat.id.as_str()), title);
    let confirm = Confirm::new(question.title, question.body, question.confirm);
    RecordAction::new(KILL, "skull", format!("Kill the agent on {title}"))
        .labelled("Kill")
        .confirm(confirm)
}

pub(crate) fn session_title(
    state: &State,
    chat: &str,
) -> String {
    let record = state
        .hosts
        .iter()
        .find_map(|slot| slot.store.chats.get(chat));
    record.map_or_else(
        || chat.to_owned(),
        |record| title_of(&record.title).to_owned(),
    )
}

fn session_tone(chat: &ChatRecord) -> Tone {
    match chat.working {
        Switch::On => Tone::Pending,
        Switch::Off => Tone::Success,
    }
}

pub(crate) fn hosts(state: &State) -> Vec<Record> {
    state.hosts.iter().map(|slot| host(state, slot)).collect()
}

fn host(
    state: &State,
    slot: &HostSlot,
) -> Record {
    let record = Record::new(slot.spec.id.as_str(), slot.spec.name.as_str())
        .icon(slot.spec.endpoint.as_ref().map_or(DESKTOP, |_| CLOUD))
        .status(link_tone(slot.link))
        .line(address_line(slot));
    if slot.spec.endpoint.is_none() {
        return record;
    }
    match state.unrevoked.get(slot.spec.id.as_str()) {
        Some(why) => record.action(remove_anyway(slot, why)),
        None => record.action(revoke(slot)),
    }
}

fn address_line(slot: &HostSlot) -> Line {
    let state = slot.link.host_state();
    let address = slot.spec.endpoint.as_deref().unwrap_or(THIS_COMPUTER);
    let text = match state {
        HostState::Up => address.to_owned(),
        _ => format!("{address} \u{b7} {}", state.label()),
    };
    if slot.spec.endpoint.is_none() {
        return Line::plain(text);
    }
    let tone = match state.tone() {
        HostTone::Offline => Tone::Error,
        HostTone::NeedsAttention => Tone::Warning,
        HostTone::Online | HostTone::Unsettled => Tone::Neutral,
    };
    Line::plain(text).mono().toned(tone)
}

fn revoke(slot: &HostSlot) -> RecordAction {
    let confirm = Confirm::new(
        format!("Revoke {}?", slot.spec.name),
        "To use it again you’ll need to run crowbar pair on it.",
        "Revoke",
    );
    RecordAction::new(REVOKE, "x", "Revoke this computer’s access to the host")
        .labelled("Revoke")
        .confirm(confirm)
}

fn remove_anyway(
    slot: &HostSlot,
    why: &str,
) -> RecordAction {
    let confirm = Confirm::new(
        format!("Remove {} anyway?", slot.spec.name),
        format!("{why} This computer forgets its key now, so nothing here can use it."),
        "Remove anyway",
    );
    RecordAction::new(
        REMOVE_ANYWAY,
        "x",
        "Remove the host from this app without revoking it",
    )
    .labelled("Remove anyway")
    .confirm(confirm)
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

    #[test]
    fn a_session_is_killed_through_develop_s_question() {
        let (state, _) = state();
        let sessions = sessions(&state);
        let confirm = Confirm::new(
            "Kill the agent on “Idea”?",
            "It stops now, even mid-turn. The conversation is kept and resumes when reopened.",
            "Kill agent",
        );
        let want = RecordAction::new(KILL, "skull", "Kill the agent on Idea")
            .labelled("Kill")
            .confirm(confirm);
        assert_eq!(sessions[0].offered_action(), Some(&want));
    }

    const REMOTE: Option<&str> = Some("box:3737");

    type HostRowCase = (
        Option<&'static str>,
        LinkState,
        (&'static str, &'static str, Tone),
    );

    const HOST_ROW_CASES: [HostRowCase; 5] = [
        (
            None,
            LinkState::Live,
            ("desktop", "This computer", Tone::Neutral),
        ),
        (
            None,
            LinkState::Down,
            ("desktop", "This computer \u{b7} Offline", Tone::Neutral),
        ),
        (
            REMOTE,
            LinkState::Live,
            ("cloud", "box:3737", Tone::Neutral),
        ),
        (
            REMOTE,
            LinkState::Connecting,
            ("cloud", "box:3737 \u{b7} Checking\u{2026}", Tone::Neutral),
        ),
        (
            REMOTE,
            LinkState::Down,
            ("cloud", "box:3737 \u{b7} Offline", Tone::Error),
        ),
    ];

    #[test]
    fn a_host_row_says_its_state_short_of_online_as_develop_does() {
        for (endpoint, link, want) in HOST_ROW_CASES {
            let (mut state, _) = state();
            state.hosts[0].spec.endpoint = endpoint.map(str::to_owned);
            state.hosts[0].link = link;
            let row = &hosts(&state)[0];
            let line = &row.lines()[0];
            assert_eq!(
                (row.icon_stem().unwrap_or(""), line.text(), line.tone()),
                want,
                "{endpoint:?} {link:?}"
            );
        }
    }
}
