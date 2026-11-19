//! Whether a chat has a turn running.

use crate::live::state::State;

pub(crate) fn of(
    state: &State,
    chat: &str,
) -> bool {
    state
        .hosts
        .iter()
        .find_map(|slot| slot.store.chats.get(chat))
        .is_some_and(|record| record.working.is_on())
}

#[cfg(test)]
mod tests {
    use super::*;
    use daemonapi::Frame;

    use crate::test_support::state;

    #[test]
    fn a_chat_is_working_only_while_its_record_says_so() {
        let (mut state, _) = state();
        let entity = |id: &str, data: &str| {
            let frame = format!(
                r#"{{"t":"entity","scope":"projects","kind":"chat","id":"{id}","v":9,"data":{data}}}"#
            );
            Frame::decode(&frame).unwrap()
        };
        state.apply("local", entity("a", r#"{"id":"a","working":true}"#));
        state.apply("local", entity("b", r#"{"id":"b","working":false}"#));
        assert!(of(&state, "a"), "working");
        assert!(!of(&state, "b"), "idle");
        assert!(!of(&state, "gone"), "unknown");
    }
}
