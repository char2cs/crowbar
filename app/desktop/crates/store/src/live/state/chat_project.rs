//! The project a chat belongs to.

use crate::live::state::State;

pub(crate) fn of(
    state: &State,
    chat: &str,
) -> Option<String> {
    state
        .hosts
        .iter()
        .find_map(|slot| slot.store.chats.get(chat))
        .map(|record| record.project_id.clone())
        .filter(|project| !project.is_empty())
}

#[cfg(test)]
mod tests {
    use super::*;
    use daemonapi::frame::decode;

    use crate::test_support::state;

    #[test]
    fn a_loaded_chat_names_its_project_and_an_unknown_or_unowned_one_none() {
        let (mut state, _) = state();
        let entity = |id: &str, data: &str| {
            let frame = format!(
                r#"{{"t":"entity","scope":"projects","kind":"chat","id":"{id}","v":9,"data":{data}}}"#
            );
            decode(&frame).unwrap()
        };
        state.apply("local", entity("a", r#"{"id":"a","projectId":"p1"}"#));
        state.apply("local", entity("b", r#"{"id":"b"}"#));
        assert_eq!(of(&state, "a").as_deref(), Some("p1"));
        assert_eq!(of(&state, "b"), None);
        assert_eq!(of(&state, "gone"), None);
    }
}
