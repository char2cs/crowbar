//! The chat, workspace and host a file operation of the Files panel runs against.

use crate::plan_error::PlanError;
use crate::state::State;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct FileTarget {
    pub(crate) host: String,
    pub(crate) chat: String,
    pub(crate) workspace: String,
}

pub(crate) fn of(state: &State) -> Result<FileTarget, PlanError> {
    let chat = state
        .panels
        .chat
        .clone()
        .ok_or(PlanError::NoWorkspaceToChange)?;
    let slot = state
        .hosts
        .iter()
        .find(|slot| slot.store.chats.get(&chat).is_some())
        .ok_or(PlanError::WorkspaceNotLoaded)?;
    let workspace = slot
        .store
        .chats
        .get(&chat)
        .map_or_default(|record| record.workspace_id.clone());
    Ok(FileTarget {
        host: slot.spec.id.clone(),
        chat,
        workspace,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::state;

    #[test]
    fn the_target_is_the_followed_chat_with_its_workspace_and_host() {
        let (mut state, _) = state();
        assert!(of(&state).is_err(), "nothing followed");
        state.panels.follow(Some("hc".into()));
        let target = of(&state).unwrap();
        assert_eq!(
            (
                target.host.as_str(),
                target.chat.as_str(),
                target.workspace.as_str()
            ),
            ("local", "hc", "hw")
        );
    }
}
