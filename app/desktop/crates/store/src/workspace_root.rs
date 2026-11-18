//! The on-disk root of the workspace the Files panel follows.

use std::path::PathBuf;

use crate::state::State;

fn non_empty(path: &str) -> Option<PathBuf> {
    Some(path)
        .filter(|path| !path.is_empty())
        .map(PathBuf::from)
}

pub(crate) fn of(state: &State) -> Option<PathBuf> {
    let chat_id = state.panels.chat.as_deref()?;
    let slot = state
        .hosts
        .iter()
        .find(|slot| slot.store.chats.get(chat_id).is_some())?;
    if slot.spec.endpoint.is_some() {
        return None;
    }
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

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::test_support::state;

    fn workspace(local_path: &str) -> crate::workspace_record::WorkspaceRecord {
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
    fn a_workspace_of_another_computer_has_no_root_here() {
        let (mut state, _) = state();
        state.panels.follow(Some("hc".into()));
        state.hosts[0].spec.endpoint = Some("far:1".into());
        assert_eq!(of(&state), None);
    }
}
