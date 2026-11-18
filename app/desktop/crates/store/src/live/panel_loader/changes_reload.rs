//! Refreshing the Changes panel when the daemon changed what it shows.

use daemonapi::Frame;
use std::sync::Arc;

use crate::live::panel_loader::load_changes;
use crate::live::state::Hub;

const OPERATION: &str = "operation";

pub(crate) fn is_operation(frame: &Frame) -> bool {
    matches!(frame, Frame::Entity { kind, .. } if kind == OPERATION)
}

pub(crate) async fn reload(hub: &Hub) {
    let followed = hub.read(|state| {
        let chat = state.panels.chat.clone()?;
        let client = state.client_of_chat(&chat)?;
        Some((chat, client))
    });
    if let Some((chat, client)) = followed {
        load_changes(hub, &client, &chat).await;
    }
}

pub(crate) fn follow(hub: &Arc<Hub>) {
    let hub = Arc::clone(hub);
    tokio::spawn(async move { reload(&hub).await });
}

#[cfg(test)]
mod tests {
    use super::*;

    use crate::test_support::state;

    fn hub_of(state: crate::live::state::State) -> Arc<Hub> {
        Hub::new(state, Arc::new(|| {}))
    }

    const STATUS: &str = r#"{"branch":"feature","files":[]}"#;

    #[test]
    fn only_operation_entities_ask_for_a_reload() {
        let operation =
            Frame::decode(r#"{"t":"entity","scope":"projects","kind":"operation","id":"o","v":2}"#);
        let chat =
            Frame::decode(r#"{"t":"entity","scope":"projects","kind":"chat","id":"c","v":2}"#);
        assert_eq!(
            (
                is_operation(&operation.unwrap()),
                is_operation(&chat.unwrap())
            ),
            (true, false)
        );
    }

    #[tokio::test]
    async fn the_followed_chats_changes_are_read_again() {
        let (mut state, daemon) = state();
        state.panels.follow(Some("hc".into()));
        let hub = hub_of(state);
        reload(&hub).await;
        assert_eq!(hub.read(|state| state.panels.changes.branch.clone()), "");
        daemon.ok("GET", "/v0/chats/hc/git/status", STATUS);
        daemon.ok("GET", "/v0/chats/hc/git/diff?staged=false", "[]");
        daemon.ok("GET", "/v0/chats/hc/git/diff?staged=true", "[]");
        reload(&hub).await;
        assert_eq!(
            hub.read(|state| state.panels.changes.branch.clone()),
            "feature"
        );
    }
}
