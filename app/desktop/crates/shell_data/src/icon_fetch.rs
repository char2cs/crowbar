//! Bringing the daemon's space images to disk when a project says it has one.

use std::path::PathBuf;
use std::sync::Arc;

use crate::call::Call;
use crate::daemon_client::DaemonClient;
use crate::hub::Hub;
use crate::state::State;

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
    let parent = path.parent()?;
    std::fs::create_dir_all(parent)
        .and_then(|()| std::fs::write(&path, bytes))
        .inspect_err(|error| tracing::warn!(%error, ?path, "a space image could not be kept"))
        .ok()?;
    Some(path)
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
}
