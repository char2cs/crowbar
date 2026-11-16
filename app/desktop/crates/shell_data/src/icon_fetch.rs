//! Bringing the daemon's space images to disk when a project says it has one.

use std::path::{Path, PathBuf};
use std::sync::Arc;

use crate::call::Call;
use crate::daemon_client::DaemonClient;
use crate::hub::Hub;
use crate::state::State;

const MAX_ICON_BYTES: usize = 8 * 1024 * 1024;

pub(crate) fn wanted(
    hub: &Hub,
    host: &str,
) -> Vec<(String, String, PathBuf)> {
    hub.edit(|state| begin_all(state, host))
}

fn begin_all(
    state: &mut State,
    host: &str,
) -> Vec<(String, String, PathBuf)> {
    projects_with_images(state, host)
        .into_iter()
        .filter_map(|(project, url)| {
            let path = state.icons.begin(&project, &url)?;
            Some((project, url, path))
        })
        .collect()
}

fn projects_with_images(
    state: &State,
    host: &str,
) -> Vec<(String, String)> {
    state
        .hosts
        .iter()
        .filter(|slot| slot.spec.id == host)
        .flat_map(|slot| slot.store.projects.values())
        .filter(|project| !project.avatar_url.is_empty())
        .map(|project| (project.id.clone(), project.avatar_url.clone()))
        .collect()
}

pub(crate) fn follow(
    hub: &Arc<Hub>,
    host: &str,
    client: &DaemonClient,
) {
    for ask in wanted(hub, host) {
        spawn_fetch(hub, client, ask);
    }
}

fn spawn_fetch(
    hub: &Arc<Hub>,
    client: &DaemonClient,
    (project, url, path): (String, String, PathBuf),
) {
    let (hub, client) = (Arc::clone(hub), client.clone());
    tokio::spawn(async move { fetch(&hub, &client, (&project, &url), path).await });
}

async fn fetch(
    hub: &Hub,
    client: &DaemonClient,
    (project, url): (&str, &str),
    path: PathBuf,
) {
    let stored = save(client, url, path).await;
    hub.edit(|state| state.icons.finish(project, url, stored));
}

async fn save(
    client: &DaemonClient,
    url: &str,
    path: PathBuf,
) -> Option<PathBuf> {
    let bytes = client
        .bytes(Call::get(url))
        .await
        .inspect_err(|error| tracing::warn!(%error, url, "a space image could not be fetched"))
        .ok()?;
    if bytes.len() > MAX_ICON_BYTES {
        tracing::warn!(url, "a space image is larger than allowed");
        return None;
    }
    tokio::task::spawn_blocking(move || write_atomically(&path, &bytes))
        .await
        .inspect_err(|error| tracing::warn!(%error, "keeping a space image failed"))
        .ok()?
}

fn write_atomically(
    path: &Path,
    bytes: &[u8],
) -> Option<PathBuf> {
    let parent = path.parent()?;
    let staging = path.with_extension("part");
    std::fs::create_dir_all(parent)
        .and_then(|()| std::fs::write(&staging, bytes))
        .and_then(|()| std::fs::rename(&staging, path))
        .inspect_err(|error| tracing::warn!(%error, ?path, "a space image could not be kept"))
        .ok()?;
    Some(path.to_owned())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::frame::decode;
    use crate::icon_cache::IconCache;
    use crate::test_support::state;

    fn hub_of(state: State) -> Arc<Hub> {
        Hub::new(state, Arc::new(|| {}))
    }

    const URL: &str = "/v0/projects/p/icon?v=2";

    #[tokio::test]
    async fn a_project_with_an_image_is_fetched_once_and_kept_as_a_file() {
        let (mut state, daemon) = state();
        let dir = tempfile::tempdir().unwrap();
        state.icons = IconCache::in_dir(dir.path().to_owned());
        state.apply(
            "local",
            decode(
                &format!(
                    r#"{{"t":"entity","scope":"projects","kind":"project","id":"p","v":9,"data":{{"id":"p","name":"A","avatarUrl":"{URL}"}}}}"#
                ),
            )
            .unwrap(),
        );
        daemon.answer(("GET", URL), 200, "PNGBYTES");
        let client = state.hosts[0].client.clone();
        let hub = hub_of(state);
        let asks = wanted(&hub, "local");
        assert_eq!(asks.len(), 1);
        assert!(wanted(&hub, "local").is_empty(), "already in flight");
        let (project, url, path) = asks.into_iter().next().unwrap();
        fetch(&hub, &client, (&project, &url), path.clone()).await;
        assert_eq!(std::fs::read(&path).unwrap(), b"PNGBYTES");
        assert!(wanted(&hub, "local").is_empty(), "held");
        let held = hub.read(|state| state.icons.file_for("p", URL).map(ToOwned::to_owned));
        assert_eq!(held, Some(path));
    }

    #[tokio::test]
    async fn a_hostile_version_in_the_avatar_url_asks_for_nothing_and_writes_nothing() {
        let (mut state, _daemon) = state();
        let dir = tempfile::tempdir().unwrap();
        let icons = dir.path().join("icons");
        state.icons = IconCache::in_dir(icons.clone());
        state.apply(
            "local",
            decode(
                r#"{"t":"entity","scope":"projects","kind":"project","id":"p","v":9,"data":{"id":"p","name":"A","avatarUrl":"/v0/projects/p/icon?v=../../escaped"}}"#,
            )
            .unwrap(),
        );
        let hub = hub_of(state);
        assert_eq!(wanted(&hub, "local").len(), 0, "no icon is requested");
        assert!(!dir.path().join("escaped").exists());
    }

    #[tokio::test]
    async fn an_image_over_the_cap_is_not_kept() {
        let (mut state, daemon) = state();
        let dir = tempfile::tempdir().unwrap();
        state.icons = IconCache::in_dir(dir.path().to_owned());
        daemon.answer(("GET", URL), 200, &"x".repeat(MAX_ICON_BYTES + 1));
        let client = state.hosts[0].client.clone();
        assert_eq!(save(&client, URL, dir.path().join("p-2")).await, None);
        assert!(!dir.path().join("p-2").exists());
    }
}
