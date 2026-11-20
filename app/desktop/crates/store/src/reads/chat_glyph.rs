//! The provider mark a chat shows at rest.

use crate::live::State;

pub(crate) fn of(
    state: &State,
    chat: &str,
) -> Option<String> {
    let (slot, record) = state
        .hosts
        .iter()
        .find_map(|slot| slot.store.chats.get(chat).map(|record| (slot, record)))?;
    slot.provider_icons
        .get(&record.active_provider_id)
        .filter(|icon| !icon.is_empty())
        .cloned()
}

#[cfg(test)]
mod tests {
    use std::collections::BTreeMap;

    use super::*;
    use crate::test_support::{state, store_of};

    const MARK: &str = "<svg>claude</svg>";

    #[test]
    fn a_chat_shows_the_mark_of_its_active_provider_once_the_host_has_listed_it() {
        let (mut state, _) = state();
        state.hosts[0].store =
            store_of(&[("chat", "c", r#"{"id":"c","activeProviderId":"claude"}"#)]);
        assert_eq!(of(&state, "c"), None, "providers not listed yet");
        let icons = BTreeMap::from([("claude".to_owned(), MARK.to_owned())]);
        state.set_provider_icons("local", icons);
        assert_eq!(of(&state, "c").as_deref(), Some(MARK));
        assert_eq!(of(&state, "missing"), None, "no such chat");
    }
}
