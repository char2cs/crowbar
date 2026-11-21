//! What the `files:` hold reads from the shared state and says to the daemon.

use std::sync::Arc;

use serde_json::json;
use tokio::sync::{Notify, mpsc};

use crate::live::panel_loader::{ReloadTarget, changes_reload};
use crate::live::state::{Hub, State};

const FILES_KIND: &str = "files";

pub(crate) fn wanted_scope(
    state: &State,
    host: &str,
) -> Option<String> {
    let chat = state.panels.chat.as_deref()?;
    let slot = state.hosts.iter().find(|slot| slot.spec.id == host)?;
    let workspace = &slot.store.chats.get(chat)?.workspace_id;
    Some(workspace)
        .filter(|workspace| !workspace.is_empty())
        .map(|workspace| format!("{FILES_KIND}:{workspace}"))
}

pub(crate) fn listed_of(
    state: &State,
    folders: Vec<String>,
) -> Vec<ReloadTarget> {
    let Some(chat) = state.panels.chat.clone() else {
        return Vec::new();
    };
    let client = state.client_of_chat(&chat);
    folders
        .into_iter()
        .filter(|folder| state.panels.listings.contains_key(folder))
        .map(|directory| ReloadTarget {
            client: client.clone(),
            chat: chat.clone(),
            directory,
        })
        .collect()
}

pub(crate) async fn load_all(
    hub: &Hub,
    targets: Vec<ReloadTarget>,
) {
    for target in targets {
        target.load(hub).await;
    }
}

pub(crate) fn send(
    outgoing: &mpsc::UnboundedSender<String>,
    (op, scope): (&str, &str),
) {
    let request = json!({"op": op, "scope": scope}).to_string();
    if outgoing.send(request).is_err() {
        tracing::debug!(scope, "the stream closed before the request went out");
    }
}

pub(crate) async fn reload_git_when_told(
    hub: Arc<Hub>,
    told: Arc<Notify>,
) {
    loop {
        told.notified().await;
        changes_reload::reload(&hub).await;
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::state;

    #[test]
    fn the_followed_chats_workspace_is_wanted_and_only_listed_folders_are_relisted() {
        let (mut state, _) = state();
        assert_eq!(wanted_scope(&state, "local"), None, "no chat followed");
        state.panels.follow(Some("hc".into()));
        assert_eq!(wanted_scope(&state, "local").as_deref(), Some("files:hw"));
        assert_eq!(
            wanted_scope(&state, "elsewhere"),
            None,
            "another host's chat"
        );
        state.panels.list("hc", "src", Vec::new());
        let folders = vec!["src".to_owned(), "lib".to_owned()];
        let listed: Vec<String> = listed_of(&state, folders)
            .into_iter()
            .map(|target| target.directory)
            .collect();
        assert_eq!(listed, ["src"]);
    }
}
