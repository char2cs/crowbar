//! Filling the Files and Changes panels for the focused chat.

pub(crate) mod changes_reload;
mod document_body;
pub(crate) mod file_target;
pub(crate) mod followed_ask;
mod git_diff_body;
mod git_status_body;
mod ignore_files;
mod line_counts;
mod listed_directory;
mod reload_target;
mod review_files_body;
mod thread_body;

pub(crate) use document_body::DocumentBody;
pub(crate) use file_target::FileTarget;
pub(crate) use reload_target::ReloadTarget;

use crate::live::held::FileEntry;
use crate::live::panel_loader::git_diff_body::GitDiffBody;
use crate::live::panel_loader::git_status_body::GitStatusBody;
use crate::live::panel_loader::line_counts::LineCounts;
use crate::live::panel_loader::listed_directory::ListedDirectory;
use crate::live::panel_loader::review_files_body::ReviewFilesBody;
use crate::live::panel_loader::thread_body::ThreadBody;
use crate::live::state::Hub;

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
    client: &daemonapi::Client,
    (chat, directory): (&str, &str),
) {
    let listed = client
        .read::<Vec<FileEntry>>(tree_path(chat, directory))
        .await;
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
    ignore_files::learn(hub, client, &listing).await;
}

pub(crate) async fn load_changes(
    hub: &Hub,
    client: &daemonapi::Client,
    chat: &str,
) {
    let answer = client
        .read::<GitStatusBody>(format!("/v0/chats/{chat}/git/status"))
        .await;
    let Ok(body) = answer.inspect_err(|error| {
        tracing::warn!(chat, %error, "git status could not be read");
    }) else {
        return;
    };
    let counts = load_counts(client, chat).await;
    let open = load_open_threads(hub, client, chat).await;
    let working = body.changes(&counts);
    let listed = load_summary(client, chat)
        .await
        .unwrap_or_else(|| working.files.clone());
    hub.edit(|state| {
        state.panels.change(chat, (working, listed));
        state
            .panels
            .staged_and_threads(chat, (body.staged_paths(), open));
    });
}

async fn load_summary(
    client: &daemonapi::Client,
    chat: &str,
) -> Option<Vec<sidebar::ChangedFile>> {
    client
        .read::<ReviewFilesBody>(format!("/v0/chats/{chat}/review/files"))
        .await
        .inspect_err(|error| tracing::warn!(chat, %error, "the branch's files could not be read"))
        .ok()
        .map(ReviewFilesBody::files)
}

async fn load_open_threads(
    hub: &Hub,
    client: &daemonapi::Client,
    chat: &str,
) -> u32 {
    let Some(path) = hub.read(|state| threads_path(state, chat)) else {
        return 0;
    };
    client
        .read::<Vec<ThreadBody>>(path)
        .await
        .inspect_err(|error| tracing::warn!(chat, %error, "review threads could not be read"))
        .map_or(0, |threads| thread_body::open_in(&threads))
}

fn threads_path(
    state: &crate::live::state::State,
    chat: &str,
) -> Option<String> {
    let record = state
        .hosts
        .iter()
        .find_map(|slot| slot.store.chats.get(chat))?;
    let scope = daemonapi::Route::scope(&record.project_id, &record.repo_id);
    Some(format!(
        "{scope}/workspaces/{}/threads",
        record.workspace_id
    ))
}

async fn load_counts(
    client: &daemonapi::Client,
    chat: &str,
) -> LineCounts {
    let mut diffs = diff_of(client, chat, "false").await;
    diffs.extend(diff_of(client, chat, "true").await);
    line_counts::of(&diffs)
}

async fn diff_of(
    client: &daemonapi::Client,
    chat: &str,
    staged: &str,
) -> Vec<GitDiffBody> {
    let path = format!("/v0/chats/{chat}/git/diff?staged={staged}");
    client
        .read::<Vec<GitDiffBody>>(path)
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

    fn first_file_counts(state: &crate::live::state::State) -> Option<(u32, u32)> {
        let file = state.panels.changes.files.first()?;
        Some((file.added, file.deleted))
    }

    fn rig() -> (Arc<Hub>, daemonapi::Client, Arc<FakeDaemon>) {
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

    fn changed_files(state: &crate::live::state::State) -> Vec<(String, GitStatus, u32, u32)> {
        let files = state.panels.changes.files.iter();
        files
            .map(|file| (file.path.clone(), file.status, file.added, file.deleted))
            .collect()
    }

    const SUMMARY: &str = r#"{"files":[{"path":"src/pricing.ts","status":"modified","additions":16,"deletions":2},{"path":"src/pricing.test.ts","status":"added","additions":18}]}"#;

    #[tokio::test]
    async fn the_changed_list_is_the_branch_summary_and_the_tree_colours_stay_the_working_trees() {
        let (hub, client, daemon) = rig();
        daemon.ok("GET", "/v0/chats/hc/git/status", STATUS);
        daemon.ok("GET", "/v0/chats/hc/git/diff?staged=false", DIFF);
        daemon.ok("GET", "/v0/chats/hc/git/diff?staged=true", "[]");
        daemon.ok("GET", "/v0/chats/hc/review/files", SUMMARY);
        load_changes(&hub, &client, "hc").await;
        let listed = hub.read(changed_files);
        assert_eq!(
            listed,
            [
                ("src/pricing.ts".to_owned(), GitStatus::Modified, 16, 2),
                ("src/pricing.test.ts".to_owned(), GitStatus::Added, 18, 0),
            ]
        );
        let colour = hub.read(|state| state.panels.statuses.get("a.txt").copied());
        assert_eq!(colour, Some(GitStatus::Untracked));
        let commit = hub.read(|state| crate::reads::merge_box(state).map(|card| card.uncommitted));
        assert_eq!(
            commit,
            Some(vec!["a.txt".to_owned()]),
            "the commit form lists the working tree"
        );
    }
}
