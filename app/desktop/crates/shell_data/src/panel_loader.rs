//! Filling the Files and Changes panels for the focused chat.

use crate::daemon_client::DaemonClient;
use crate::daemon_read::read;
use crate::file_entry::FileEntry;
use crate::git_diff_body::GitDiffBody;
use crate::git_status_body::GitStatusBody;
use crate::hub::Hub;
use crate::line_counts::{self, LineCounts};
use crate::listed_directory::ListedDirectory;
use crate::thread_body::{self, ThreadBody};

const ROOT_QUERY: &str = ".";

pub(crate) fn tree_path(
    chat: &str,
    directory: &str,
) -> String {
    let dir = Some(directory)
        .filter(|name| !name.is_empty())
        .unwrap_or(ROOT_QUERY);
    format!("/v0/chats/{chat}/files/tree?path={dir}")
}

pub(crate) async fn load_directory(
    hub: &Hub,
    client: &DaemonClient,
    (chat, directory): (&str, &str),
) {
    let listed = read::<Vec<FileEntry>>(client, tree_path(chat, directory)).await;
    let Ok(entries) = listed.inspect_err(|error| {
        tracing::warn!(chat, directory, %error, "a directory could not be listed");
    }) else {
        return;
    };
    let held = entries.clone();
    hub.edit(|state| state.panels.list(chat, directory, entries));
    let listing = ListedDirectory {
        chat,
        directory,
        entries: &held,
    };
    crate::ignore_files::learn(hub, client, &listing).await;
}

pub(crate) async fn load_changes(
    hub: &Hub,
    client: &DaemonClient,
    chat: &str,
) {
    let answer = read::<GitStatusBody>(client, format!("/v0/chats/{chat}/git/status")).await;
    let Ok(body) = answer.inspect_err(|error| {
        tracing::warn!(chat, %error, "git status could not be read");
    }) else {
        return;
    };
    let counts = load_counts(client, chat).await;
    let open = load_open_threads(hub, client, chat).await;
    hub.edit(|state| {
        state.panels.change(chat, body.changes(&counts));
        state
            .panels
            .staged_and_threads(chat, (body.staged_paths(), open));
    });
}

async fn load_open_threads(
    hub: &Hub,
    client: &DaemonClient,
    chat: &str,
) -> u32 {
    let Some(path) = hub.read(|state| threads_path(state, chat)) else {
        return 0;
    };
    read::<Vec<ThreadBody>>(client, path)
        .await
        .inspect_err(|error| tracing::warn!(chat, %error, "review threads could not be read"))
        .map_or(0, |threads| thread_body::open_in(&threads))
}

fn threads_path(
    state: &crate::state::State,
    chat: &str,
) -> Option<String> {
    let record = state
        .hosts
        .iter()
        .find_map(|slot| slot.store.chats.get(chat))?;
    let scope = crate::routes::scope(&record.project_id, &record.repo_id);
    Some(format!(
        "{scope}/workspaces/{}/threads",
        record.workspace_id
    ))
}

async fn load_counts(
    client: &DaemonClient,
    chat: &str,
) -> LineCounts {
    let mut diffs = diff_of(client, chat, "false").await;
    diffs.extend(diff_of(client, chat, "true").await);
    line_counts::of(&diffs)
}

async fn diff_of(
    client: &DaemonClient,
    chat: &str,
    staged: &str,
) -> Vec<GitDiffBody> {
    let path = format!("/v0/chats/{chat}/git/diff?staged={staged}");
    read::<Vec<GitDiffBody>>(client, path)
        .await
        .inspect_err(|error| tracing::warn!(chat, %error, "the line counts could not be read"))
        .unwrap_or_default()
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use sidebar::GitStatus;

    use super::*;
    use crate::test_support::{FakeDaemon, state};

    fn first_file_counts(state: &crate::state::State) -> Option<(u32, u32)> {
        let file = state.panels.changes.files.first()?;
        Some((file.added, file.deleted))
    }

    fn rig() -> (Arc<Hub>, DaemonClient, Arc<FakeDaemon>) {
        let (mut state, daemon) = state();
        state.panels.follow(Some("hc".into()));
        let client = state.hosts[0].client.clone();
        (Hub::new(state, Arc::new(|| {})), client, daemon)
    }

    const TREE: &str = r#"[{"name":"a.txt","path":"a.txt","type":"file"}]"#;
    const DIFF: &str = r#"[{"file_path":"a.txt","additions":3,"deletions":1}]"#;
    const STATUS: &str = r#"{"branch":"main","files":[{"path":"a.txt","status":"untracked"}]}"#;

    #[tokio::test]
    async fn the_panels_fill_from_the_daemon_and_ignore_other_chats_and_refusals() {
        let (hub, client, daemon) = rig();
        load_directory(&hub, &client, ("hc", "")).await;
        load_changes(&hub, &client, "hc").await;
        assert!(
            hub.read(|state| state.panels.listings.is_empty()),
            "a refused read changes nothing"
        );
        daemon.ok("GET", "/v0/chats/hc/files/tree?path=.", TREE);
        daemon.ok("GET", "/v0/chats/hc/git/status", STATUS);
        daemon.ok("GET", "/v0/chats/hc/git/diff?staged=false", DIFF);
        daemon.ok("GET", "/v0/chats/hc/git/diff?staged=true", "[]");
        load_directory(&hub, &client, ("other", "")).await;
        load_directory(&hub, &client, ("hc", "")).await;
        load_changes(&hub, &client, "hc").await;
        let listed = hub.read(|state| state.panels.listings.len());
        let branch = hub.read(|state| state.panels.changes.branch.clone());
        let status = hub.read(|state| state.panels.statuses.get("a.txt").copied());
        let lines = hub.read(first_file_counts);
        assert_eq!(
            (listed, branch.as_str(), status, lines),
            (1, "main", Some(GitStatus::Untracked), Some((3, 1)))
        );
    }
}
