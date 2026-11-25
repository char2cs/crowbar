//! Bringing the daemon's space and repository images to disk when a record says it has one.

use daemonapi::Call;
use std::path::{Path, PathBuf};
use std::sync::Arc;

use crate::live::Entities;
use crate::live::fleet::host_stream::icon_ask::IconAsk;
use crate::live::state::Hub;
use crate::live::state::State;

const MAX_ICON_BYTES: usize = 8 * 1024 * 1024;

pub(crate) fn wanted(
    hub: &Hub,
    host: &str,
) -> Vec<IconAsk> {
    hub.edit(|state| begin_all(state, host))
}

fn begin_all(
    state: &mut State,
    host: &str,
) -> Vec<IconAsk> {
    with_images(state, host)
        .into_iter()
        .filter_map(|(project, url)| {
            let path = state.icons.begin(&project, &url)?;
            Some(IconAsk { project, url, path })
        })
        .collect()
}

fn with_images(
    state: &State,
    host: &str,
) -> Vec<(String, String)> {
    state
        .hosts
        .iter()
        .filter(|slot| slot.spec.id == host)
        .flat_map(|slot| images_of(&slot.store))
        .collect()
}

fn images_of(store: &Entities) -> Vec<(String, String)> {
    let projects = store.projects.values().map(|p| (&p.id, &p.avatar_url));
    let repos = store.repos.values().map(|r| (&r.id, &r.avatar_url));
    projects
        .chain(repos)
        .filter(|(_, url)| !url.is_empty())
        .map(|(id, url)| (id.clone(), url.clone()))
        .collect()
}

pub(crate) fn follow(
    hub: &Arc<Hub>,
    host: &str,
    client: &daemonapi::Client,
) {
    for ask in wanted(hub, host) {
        spawn_fetch(hub, client, ask);
    }
}

fn spawn_fetch(
    hub: &Arc<Hub>,
    client: &daemonapi::Client,
    ask: IconAsk,
) {
    let (hub, client) = (Arc::clone(hub), client.clone());
    tokio::spawn(async move { fetch(&hub, &client, ask).await });
}

async fn fetch(
    hub: &Hub,
    client: &daemonapi::Client,
    ask: IconAsk,
) {
    let IconAsk { project, url, path } = ask;
    let stored = save(client, &url, path).await;
    hub.edit(|state| state.icons.finish(&project, &url, stored));
}

async fn save(
    client: &daemonapi::Client,
    url: &str,
    path: PathBuf,
) -> Option<PathBuf> {
    let bytes = download(client, url).await?;
    if bytes.len() > MAX_ICON_BYTES {
        warn_too_large(url);
        return None;
    }
    keep(path, bytes).await
}

async fn download(
    client: &daemonapi::Client,
    url: &str,
) -> Option<Vec<u8>> {
    client
        .bytes(Call::get(url))
        .await
        .inspect_err(|error| warn_unfetched(error, url))
        .ok()
}

fn warn_unfetched(
    error: &daemonapi::Error,
    url: &str,
) {
    tracing::warn!(%error, url, "a space image could not be fetched");
}

fn warn_too_large(url: &str) {
    tracing::warn!(url, "a space image is larger than allowed");
}

async fn keep(
    path: PathBuf,
    bytes: Vec<u8>,
) -> Option<PathBuf> {
    tokio::task::spawn_blocking(move || write_atomically(&path, &bytes))
        .await
        .inspect_err(warn_unkept)
        .ok()?
}

fn warn_unkept(error: &tokio::task::JoinError) {
    tracing::warn!(%error, "keeping a space image failed");
}

fn write_atomically(
    path: &Path,
    bytes: &[u8],
) -> Option<PathBuf> {
    atomicfile::write(path, bytes)
        .inspect_err(|error| tracing::warn!(%error, ?path, "a space image could not be kept"))
        .ok()?;
    Some(path.to_owned())
}

#[cfg(test)]
mod tests {
    use super::*;
    use daemonapi::Frame;

    use crate::live::held::IconCache;
    use crate::test_support::state;

    fn hub_of(state: State) -> Arc<Hub> {
        Hub::new(state, Arc::new(|| {}))
    }

    const URL: &str = "/v0/projects/p/icon?v=2";

    #[tokio::test]
    #[ignore = "real-infra: writes icons through atomicfile into a real directory"]
    async fn a_project_with_an_image_is_fetched_once_and_kept_as_a_file() {
        let (mut state, daemon) = state();
        let dir = tempfile::tempdir().unwrap();
        state.icons = IconCache::in_dir(dir.path().to_owned());
        state.apply(
            "local",
            Frame::decode(
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
        let ask = asks.into_iter().next().unwrap();
        let path = ask.path.clone();
        fetch(&hub, &client, ask).await;
        assert_eq!(std::fs::read(&path).unwrap(), b"PNGBYTES");
        assert!(wanted(&hub, "local").is_empty(), "held");
        let held = hub.read(|state| state.icons.file_for("p", URL).map(ToOwned::to_owned));
        assert_eq!(held, Some(path));
    }

    fn asks_for(version: &str) -> usize {
        let (mut state, _daemon) = state();
        state.icons = IconCache::in_dir(PathBuf::from("/crowbar-test-missing/icons"));
        let frame = format!(
            r#"{{"t":"entity","scope":"projects","kind":"project","id":"p","v":9,"data":{{"id":"p","name":"A","avatarUrl":"/v0/projects/p/icon?v={version}"}}}}"#
        );
        state.apply("local", Frame::decode(&frame).unwrap());
        wanted(&hub_of(state), "local").len()
    }

    #[test]
    fn a_repo_with_an_image_is_asked_for_as_a_space_is() {
        let (mut state, _daemon) = state();
        state.icons = IconCache::in_dir(PathBuf::from("/crowbar-test-missing/icons"));
        let url = "/v0/projects/p/repos/r/icon?v=3";
        let frame = format!(
            r#"{{"t":"entity","scope":"projects","kind":"repo","id":"r","v":9,"data":{{"id":"r","projectId":"p","name":"crowbar","avatarUrl":"{url}"}}}}"#
        );
        state.apply("local", Frame::decode(&frame).unwrap());
        let asks = wanted(&hub_of(state), "local");
        let asked: Vec<(&str, &str)> = asks
            .iter()
            .map(|ask| (ask.project.as_str(), ask.url.as_str()))
            .collect();
        assert_eq!(asked, [("r", url)]);
    }

    #[test]
    #[ignore = "real-infra: writes icons through atomicfile into a real directory"]
    fn a_hostile_version_in_the_avatar_url_asks_for_nothing() {
        let cases = ["../../escaped", "a/b"];
        for version in cases {
            assert_eq!(asks_for(version), 0, "{version}");
        }
    }

    #[tokio::test]
    #[ignore = "real-infra: writes icons through atomicfile into a real directory"]
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
