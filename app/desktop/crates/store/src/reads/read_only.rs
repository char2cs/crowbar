use crate::live::State;
use crate::rows::workspace_view;

pub(crate) fn of(
    state: &State,
    chat: &str,
) -> bool {
    state.hosts.iter().any(|slot| {
        let workspace = slot
            .store
            .chats
            .get(chat)
            .and_then(|record| slot.store.workspaces.get(&record.workspace_id));
        workspace.is_some_and(workspace_view::is_locked)
    })
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::live::WorkspaceRecord;
    use crate::test_support::state;

    #[test]
    fn only_a_chat_whose_workspace_is_locked_is_read_only() {
        let (mut state, _) = state();
        let locked: WorkspaceRecord =
            serde_json::from_value(json!({"id": "b", "status": "locked"})).unwrap();
        state.hosts[0].store.workspaces.put("b", 99, locked);
        let cases = [
            ("the owner of the locked branch", "bo", true),
            ("a thread on it", "t", true),
            ("the repository's checkout", "rdo", false),
            ("an unknown chat", "nope", false),
        ];
        for (name, chat, expected) in cases {
            assert_eq!(of(&state, chat), expected, "{name}");
        }
    }
}
