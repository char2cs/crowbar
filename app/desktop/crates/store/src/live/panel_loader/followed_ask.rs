//! Asking the followed chat's daemon for its panels, once its host knows the chat.

use std::sync::Arc;

use tokio::runtime::Handle;

use crate::live::panel_loader::{load_changes, load_directory};
use crate::live::state::{Hub, State};

fn begin(state: &mut State) -> Option<(String, daemonapi::Client)> {
    let chat = state.panels.chat.clone()?;
    let client = state.client_of_chat(&chat)?;
    state.panels.ask()?;
    Some((chat, client))
}

pub(crate) fn ask(
    hub: &Arc<Hub>,
    handle: &Handle,
) {
    let Some((chat, client)) = hub.edit(begin) else {
        return;
    };
    let hub = Arc::clone(hub);
    handle.spawn(async move {
        load_directory(&hub, &client, (&chat, "")).await;
        load_changes(&hub, &client, &chat).await;
    });
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::state;

    #[test]
    fn a_followed_chat_is_asked_for_once_its_host_knows_it_and_only_once() {
        let (mut state, _) = state();
        state.panels.follow(Some("unknown".into()));
        assert!(begin(&mut state).is_none(), "no host knows the chat yet");
        state.panels.follow(Some("hc".into()));
        let asked = begin(&mut state).map(|(chat, _)| chat);
        assert_eq!(asked.as_deref(), Some("hc"));
        assert!(begin(&mut state).is_none(), "asked already");
    }
}
