//! Bringing the daemon's space images to disk when a project says it has one.

use daemonapi::{Call, DaemonClient};
use std::path::{Path, PathBuf};
use std::sync::Arc;

use crate::hub::Hub;
use crate::icon_ask::IconAsk;
use crate::state::State;

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
    projects_with_images(state, host)
        .into_iter()
        .filter_map(|(project, url)| {
            let path = state.icons.begin(&project, &url)?;
            Some(IconAsk { project, url, path })
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
    ask: IconAsk,
) {
    let (hub, client) = (Arc::clone(hub), client.clone());
    tokio::spawn(async move { fetch(&hub, &client, ask).await });
}

async fn fetch(
    hub: &Hub,
    client: &DaemonClient,
    ask: IconAsk,
) {
    let IconAsk { project, url, path } = ask;
    let stored = save(client, &url, path).await;
    hub.edit(|state| state.icons.finish(&project, &url, stored));
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
    atomicfile::write(path, bytes)
        .inspect_err(|error| tracing::warn!(%error, ?path, "a space image could not be kept"))
        .ok()?;
    Some(path.to_owned())
}

#[cfg(test)]
mod tests {
    use super::*;
    use daemonapi::frame::decode;

    use crate::icon_cache::IconCache;
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
        state.apply("local", decode(&frame).unwrap());
        wanted(&hub_of(state), "local").len()
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
