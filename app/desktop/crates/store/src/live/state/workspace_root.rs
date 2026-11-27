use std::path::PathBuf;

use crate::live::state::State;

fn non_empty(path: &str) -> Option<PathBuf> {
    Some(path)
        .filter(|path| !path.is_empty())
        .map(PathBuf::from)
}

pub(crate) fn of(state: &State) -> Option<PathBuf> {
    let chat_id = state.panels.chat.as_deref()?;
    let remote = host_of(state, chat_id)?.spec.endpoint.is_some();
    if remote {
        return None;
    }
    of_chat(state, chat_id)
}

pub(crate) fn of_chat(
    state: &State,
    chat_id: &str,
) -> Option<PathBuf> {
    let slot = host_of(state, chat_id)?;
    let chat = slot.store.chats.get(chat_id)?;
    let own = slot
        .store
        .workspaces
        .get(&chat.workspace_id)
        .and_then(|workspace| non_empty(&workspace.local_path));
    own.or_else(|| {
        let project = slot.store.projects.get(&chat.project_id)?;
        non_empty(&project.path)
    })
}

pub(crate) fn remote_host_of(
    state: &State,
    chat_id: &str,
) -> Option<String> {
    let slot = host_of(state, chat_id)?;
    slot.spec.endpoint.as_ref().map(|_| slot.spec.name.clone())
}

pub(crate) fn host_id_of(
    state: &State,
    chat_id: &str,
) -> Option<String> {
    host_of(state, chat_id).map(|slot| slot.spec.id.clone())
}

fn host_of<'a>(
    state: &'a State,
    chat_id: &str,
) -> Option<&'a crate::live::held::HostSlot> {
    state
        .hosts
        .iter()
        .find(|slot| slot.store.chats.get(chat_id).is_some())
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::test_support::state;

    fn workspace(local_path: &str) -> crate::live::held::WorkspaceRecord {
        serde_json::from_value(json!({"id": "hw", "projectId": "p", "localPath": local_path}))
            .unwrap()
    }

    #[test]
    fn the_root_is_the_followed_workspaces_checkout_else_its_space_folder() {
        let (mut state, _) = state();
        assert_eq!(of(&state), None, "no chat is followed");
        state.panels.follow(Some("hc".into()));
        state.hosts[0]
            .store
            .workspaces
            .put("hw", 99, workspace("/work/tree"));
        assert_eq!(of(&state), Some(PathBuf::from("/work/tree")));
        state.hosts[0]
            .store
            .workspaces
            .put("hw", 100, workspace(""));
        state.hosts[0].store.projects.put(
            "p",
            100,
            serde_json::from_value(json!({"id": "p", "path": "/home/me/space"})).unwrap(),
        );
        assert_eq!(of(&state), Some(PathBuf::from("/home/me/space")));
    }

    #[test]
    fn a_chat_on_another_computer_has_no_root_here_but_keeps_its_own_and_names_its_host() {
        let (mut state, _) = state();
        state.panels.follow(Some("hc".into()));
        state.hosts[0]
            .store
            .workspaces
            .put("hw", 99, workspace("/far/tree"));
        assert_eq!(remote_host_of(&state, "hc"), None, "this computer");
        assert_eq!(host_id_of(&state, "hc").as_deref(), Some("local"));
        state.hosts[0].spec.endpoint = Some("far:1".into());
        assert_eq!(of(&state), None, "not a folder of this computer");
        assert_eq!(of_chat(&state, "hc"), Some(PathBuf::from("/far/tree")));
        let name = state.hosts[0].spec.name.clone();
        assert_eq!(remote_host_of(&state, "hc"), Some(name));
        assert_eq!(of_chat(&state, "gone"), None);
    }
}
