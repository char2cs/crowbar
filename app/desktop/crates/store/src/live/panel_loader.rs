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
mod panel_loader_event;
mod reload_target;
mod review_files_body;
mod thread_body;

pub(crate) use document_body::DocumentBody;
pub(crate) use file_target::FileTarget;
pub(crate) use reload_target::ReloadTarget;

use crate::journal_sink::emit;
use crate::live::held::FileEntry;
use crate::live::held::OpenThreads;
use crate::live::panel_loader::git_diff_body::GitDiffBody;
use crate::live::panel_loader::git_status_body::GitStatusBody;
use crate::live::panel_loader::line_counts::LineCounts;
use crate::live::panel_loader::listed_directory::ListedDirectory;
use crate::live::panel_loader::panel_loader_event::PanelLoaderEvent;
use crate::live::panel_loader::review_files_body::ReviewFilesBody;
use crate::live::panel_loader::thread_body::ThreadBody;
use crate::live::state::Hub;
use crate::live::{Outcome, OutcomeSink};

const ROOT_QUERY: &str = ".";
const LOAD_TREE: &str = "Load the file tree";
const LOAD_FOLDER: &str = "Load the folder";

pub(crate) fn tree_path(
    chat: &str,
    directory: &str,
) -> String {
    let dir = Some(directory)
        .filter(|name| !name.is_empty())
        .unwrap_or(ROOT_QUERY);
    format!("/v0/chats/{chat}/files/tree?path={dir}")
}

async fn list_into(
    hub: &Hub,
    client: &daemonapi::Client,
    (chat, directory): (&str, &str),
) -> daemonapi::Result<()> {
    let entries = client
        .read::<Vec<FileEntry>>(tree_path(chat, directory))
        .await?;
    let held = entries.clone();
    hub.edit(|state| state.panels.list(chat, directory, entries));
    let listing = ListedDirectory {
        chat,
        directory,
        entries: &held,
    };
    ignore_files::learn(hub, client, &listing).await;
    Ok(())
}

pub(crate) async fn load_directory(
    hub: &Hub,
    client: &daemonapi::Client,
    (chat, directory): (&str, &str),
) {
    let listed = list_into(hub, client, (chat, directory)).await;
    if let Err(error) = listed {
        tracing::warn!(chat, directory, %error, "a directory could not be listed");
    }
}

pub(crate) async fn load_reported(
    hub: &Hub,
    (client, outcome): (&daemonapi::Client, &OutcomeSink),
    at: (&str, &str),
) {
    let (chat, directory) = at;
    let Err(error) = list_into(hub, client, at).await else {
        return;
    };
    tracing::warn!(chat, directory, %error, "a directory could not be listed");
    if directory.is_empty() {
        hub.edit(|state| state.panels.list(chat, directory, Vec::new()));
    }
    outcome(Outcome::Refused {
        title: load_action(directory).to_owned(),
        reason: error.to_string(),
    });
}

// SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/features/workspace/stores/hooks/use-workspace-file-tree.ts
fn load_action(directory: &str) -> &'static str {
    if directory.is_empty() {
        return LOAD_TREE;
    }
    LOAD_FOLDER
}

pub(crate) async fn load_changes(
    hub: &Hub,
    client: &daemonapi::Client,
    chat: &str,
) {
    let answer = client
        .read::<GitStatusBody>(format!("/v0/chats/{chat}/git/status"))
        .await
        .inspect_err(|error| emit(&PanelLoaderEvent::StatusUnread { chat, error }));
    let body = match answer {
        Ok(body) => body,
        Err(error) => return hub.edit(|state| state.panels.fail(chat, error.to_string())),
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
) -> OpenThreads {
    let Some(path) = hub.read(|state| threads_path(state, chat)) else {
        return OpenThreads::default();
    };
    client
        .read::<Vec<ThreadBody>>(path)
        .await
        .inspect_err(|error| tracing::warn!(chat, %error, "review threads could not be read"))
        .map_or_else(
            |_| OpenThreads::default(),
            |threads| thread_body::open_in(&threads),
        )
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
        daemon.answer(("GET", "/v0/chats/hc/git/status"), 500, NOT_A_REPO);
        load_changes(&hub, &client, "hc").await;
        let changes = hub.read(crate::reads::changes);
        let colours = hub.read(|state| state.panels.statuses.len());
        assert_eq!(
            (changes.error.as_deref(), changes.files.len(), colours),
            (Some("git: status: not a git repository: /gone"), 0, 0),
            "a status git cannot read leaves its error and no files"
        );
    }

    type Heard = Arc<std::sync::Mutex<Vec<Outcome>>>;

    fn heard() -> (OutcomeSink, Heard) {
        let heard = Heard::default();
        let keep = Arc::clone(&heard);
        let sink: OutcomeSink = Arc::new(move |outcome| keep.lock().unwrap().push(outcome));
        (sink, heard)
    }

    fn titles(heard: &Heard) -> Vec<(String, String)> {
        let heard = heard.lock().unwrap();
        let refused = heard.iter().filter_map(|outcome| match outcome {
            Outcome::Refused { title, reason } => Some((title.clone(), reason.clone())),
            _ => None,
        });
        refused.collect()
    }

    async fn failed(
        hub: &Hub,
        client: &daemonapi::Client,
        directory: &str,
    ) -> Vec<(String, String)> {
        let (sink, heard) = heard();
        load_reported(hub, (client, &sink), ("hc", directory)).await;
        titles(&heard)
    }

    const INJECTED: &str = r#"{"success":false,"error":"rig fault: injected 500"}"#;

    const FAILED_LISTING_CASES: [(&str, &str, &str); 2] = [
        ("", "/v0/chats/hc/files/tree?path=.", "Load the file tree"),
        ("src", "/v0/chats/hc/files/tree?path=src", "Load the folder"),
    ];

    #[test]
    fn a_listing_that_fails_is_toasted_and_an_unlisted_root_reads_as_an_empty_folder() {
        let runtime = tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .unwrap();
        let (hub, client, daemon) = rig();
        hub.edit(|state| state.panels.ask());
        for (directory, route, title) in FAILED_LISTING_CASES {
            daemon.answer(("GET", route), 500, INJECTED);
            let heard = runtime.block_on(failed(&hub, &client, directory));
            let said = (title.to_owned(), "rig fault: injected 500".to_owned());
            assert_eq!(heard, [said], "{directory:?}");
        }
        let phase = hub.read(crate::reads::files::phase);
        assert_eq!(phase, sidebar::FolderPhase::Listed);
    }

    const NOT_A_REPO: &str =
        r#"{"success":false,"error":"git: status: not a git repository: /gone"}"#;

    fn changed_files(state: &crate::live::state::State) -> Vec<(String, GitStatus, u32, u32)> {
        let files = state.panels.changes.files.iter();
        files
            .map(|file| (file.path.clone(), file.status, file.added, file.deleted))
            .collect()
    }

    const THREADS: &str = r#"[{"filePath":"src/pricing.ts"},{"filePath":"src/pricing.ts"},{"filePath":""},{"filePath":"a.txt","resolved":true}]"#;

    const SUMMARY: &str = r#"{"files":[{"path":"src/pricing.ts","status":"modified","additions":16,"deletions":2},{"path":"src/pricing.test.ts","status":"added","additions":18}]}"#;

    #[tokio::test]
    async fn the_changed_list_is_the_branch_summary_and_the_tree_colours_stay_the_working_trees() {
        let (hub, client, daemon) = rig();
        daemon.ok("GET", "/v0/chats/hc/git/status", STATUS);
        daemon.ok("GET", "/v0/chats/hc/git/diff?staged=false", DIFF);
        daemon.ok("GET", "/v0/chats/hc/git/diff?staged=true", "[]");
        daemon.ok("GET", "/v0/chats/hc/review/files", SUMMARY);
        let threads = hub.read(|state| threads_path(state, "hc"));
        daemon.ok("GET", &threads.unwrap_or_default(), THREADS);
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
        let per_file = hub.read(|state| crate::reads::changes(state).threads);
        let open = std::collections::BTreeMap::from([("src/pricing.ts".to_owned(), 2)]);
        assert_eq!(per_file, open, "a file counts its open threads");
    }
}
