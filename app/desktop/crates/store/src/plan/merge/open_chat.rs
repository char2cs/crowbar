//! The chat a panel follows and the host that holds it.

use crate::live::PlanError;
use crate::live::State;

pub(crate) struct OpenChat<'a> {
    pub(crate) chat: &'a str,
    pub(crate) host: &'a str,
}

impl<'a> OpenChat<'a> {
    pub(crate) fn of(state: &'a State) -> Result<Self, PlanError> {
        let chat = state
            .panels
            .chat
            .as_deref()
            .ok_or(PlanError::NoWorkspaceOpen)?;
        let host = state
            .hosts
            .iter()
            .find(|slot| slot.store.chats.get(chat).is_some())
            .map(|slot| slot.spec.id.as_str())
            .ok_or(PlanError::WorkspaceNotLoaded)?;
        Ok(Self { chat, host })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::state;

    #[test]
    fn a_followed_chat_names_its_host_and_none_is_refused() {
        let (mut state, _) = state();
        assert!(OpenChat::of(&state).is_err());
        state.panels.follow(Some("hc".into()));
        let open = OpenChat::of(&state).unwrap();
        assert_eq!(open.chat, "hc");
        assert_eq!(open.host, state.hosts[0].spec.id);
    }
}
