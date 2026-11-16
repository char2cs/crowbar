//! Reading the `.gitignore` files that a directory listing shows.

use crate::daemon_client::DaemonClient;
use crate::daemon_read::read;
use crate::document_body::DocumentBody;
use crate::document_route;
use crate::file_entry::FileEntry;
use crate::file_target;
use crate::hub::Hub;

const IGNORE_FILE: &str = ".gitignore";

fn listed(entries: &[FileEntry]) -> Option<&FileEntry> {
    entries
        .iter()
        .find(|entry| entry.name == IGNORE_FILE && !entry.is_directory())
}

pub(crate) async fn learn(
    hub: &Hub,
    client: &DaemonClient,
    (chat, directory): (&str, &str),
    entries: &[FileEntry],
) {
    let Some(file) = listed(entries) else {
        return;
    };
    let target = hub.read(file_target::of);
    let Some(workspace) = target.ok().filter(|held| held.chat == chat) else {
        return;
    };
    let route = document_route::content(&workspace.workspace, &file.path);
    let Ok(body) = read::<DocumentBody>(client, route)
        .await
        .inspect_err(|error| tracing::warn!(chat, %error, "a .gitignore could not be read"))
    else {
        return;
    };
    hub.edit(|state| state.panels.learn_ignore(chat, directory, &body.content));
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;
    use crate::entry_kind::EntryKind;
    use crate::test_support::state;

    fn entry(
        name: &str,
        kind: &str,
    ) -> FileEntry {
        FileEntry {
            name: name.into(),
            path: format!("sub/{name}"),
            kind: kind.into(),
        }
    }

    fn quiet(state: crate::state::State) -> Arc<Hub> {
        Hub::new(state, Arc::new(|| {}))
    }

    fn hides_log(hub: &Hub) -> bool {
        hub.read(|state| state.panels.ignores.ignores("sub/a.log", EntryKind::File))
    }

    #[test]
    fn only_a_file_named_gitignore_is_read() {
        let cases = [
            (vec![entry(".gitignore", "file")], true),
            (vec![entry(".gitignore", "directory")], false),
            (vec![entry("a.txt", "file")], false),
            (Vec::new(), false),
        ];
        for (entries, found) in cases {
            assert_eq!(listed(&entries).is_some(), found, "{entries:?}");
        }
    }

    #[tokio::test]
    async fn the_rules_of_a_listed_file_apply_to_the_followed_workspace_only() {
        let (mut state, daemon) = state();
        state.panels.follow(Some("hc".into()));
        let client = state.hosts[0].client.clone();
        let hub = quiet(state);
        let route = "/v0/documents/content?workspace=hw&path=sub/.gitignore";
        daemon.ok("GET", route, r#"{"content":"*.log"}"#);
        let entries = [entry(".gitignore", "file")];
        learn(&hub, &client, ("other", "sub"), &entries).await;
        assert!(!hides_log(&hub));
        learn(&hub, &client, ("hc", "sub"), &entries).await;
        assert!(hides_log(&hub));
    }
}
