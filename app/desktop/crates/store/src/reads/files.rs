//! What the Files panel menu asks of the followed workspace.

use sidebar::{Command, FolderPhase};

use crate::live::HostSlot;
use crate::live::State;
use crate::live::WorkspaceRecord;

const LOCKED: &str = "locked";

fn host_of_followed(state: &State) -> Option<&HostSlot> {
    let chat = state.panels.chat.as_deref()?;
    state
        .hosts
        .iter()
        .find(|slot| slot.store.chats.get(chat).is_some())
}

pub(crate) fn followed_workspace(state: &State) -> Option<&WorkspaceRecord> {
    let slot = host_of_followed(state)?;
    let chat = slot.store.chats.get(state.panels.chat.as_deref()?)?;
    slot.store.workspaces.get(&chat.workspace_id)
}

pub(crate) fn root(state: &State) -> Option<String> {
    crate::live::workspace_root::of(state).map(|path| path.to_string_lossy().into_owned())
}

pub(crate) fn phase(state: &State) -> FolderPhase {
    if followed_workspace(state).is_none() {
        return FolderPhase::NoFolder;
    }
    if state.panels.listing_root() {
        return FolderPhase::Loading;
    }
    FolderPhase::Listed
}

pub(crate) fn locked(state: &State) -> bool {
    followed_workspace(state).is_some_and(|workspace| workspace.status == LOCKED)
}

pub(crate) fn local(state: &State) -> bool {
    host_of_followed(state).is_none_or(|slot| slot.spec.endpoint.is_none())
}

pub(crate) fn shown(command: &Command) -> bool {
    !matches!(
        command,
        Command::GenerateImage { .. } | Command::RemoveFolderFromWorkspace { .. }
    )
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::test_support::state;

    fn locked_workspace() -> WorkspaceRecord {
        serde_json::from_value(json!({"id": "hw", "status": "locked", "localPath": "/w"})).unwrap()
    }

    #[test]
    fn every_entry_is_offered_but_the_ones_without_a_daemon_route() {
        let image = Command::GenerateImage { path: "a".into() };
        let remove = Command::RemoveFolderFromWorkspace { path: "a".into() };
        let upload = Command::UploadFile { path: "a".into() };
        assert_eq!((shown(&image), shown(&remove)), (false, false));
        assert!(shown(&upload) && shown(&Command::AddFolderToWorkspace));
    }

    #[test]
    fn the_menu_reads_the_followed_workspaces_root_lock_and_host() {
        let (mut state, _) = state();
        assert_eq!(
            (root(&state), locked(&state), local(&state)),
            (None, false, true)
        );
        state.panels.follow(Some("hc".into()));
        state.hosts[0]
            .store
            .workspaces
            .put("hw", 99, locked_workspace());
        assert_eq!(
            (root(&state), locked(&state), local(&state)),
            (Some("/w".to_owned()), true, true)
        );
        state.hosts[0].spec.endpoint = Some("far:1".into());
        assert!(!local(&state));
    }
}
