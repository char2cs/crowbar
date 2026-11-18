//! What the settings core reads from the daemon: the agents, their checks and the permission level.

use std::sync::Arc;

use daemonapi::Call;
use settings::{Change, Key, Value};

use crate::link_state::LinkState;
use crate::loadable::Loadable;
use crate::outcome::Outcome;
use crate::permission_key::{PERMISSION, PERMISSION_PATH};
use crate::settings_core::{AGENTS, DESCRIPTORS, SettingsCore, locked};
use crate::state::State;
use crate::{descriptors, providers};

fn local_link(state: &State) -> Option<LinkState> {
    state
        .hosts
        .iter()
        .find(|slot| slot.spec.endpoint.is_none())
        .map(|slot| slot.link)
}

fn answered(state: &State) -> Option<LinkState> {
    local_link(state).filter(|link| *link != LinkState::Connecting)
}

fn live(state: &State) -> Option<LinkState> {
    local_link(state).filter(|link| *link == LinkState::Live)
}

impl SettingsCore {
    pub(crate) async fn when_the_daemon_answers(&self) {
        let reached = self.hub.until(answered).await;
        if reached == Some(LinkState::Live) {
            return;
        }
        *locked(&self.agents) = Loadable::Failed;
        *locked(&self.descriptors) = Loadable::Failed;
        self.push(Change::Records(AGENTS.to_owned()));
        self.push(Change::Records(DESCRIPTORS.to_owned()));
        self.hub.until(live).await;
    }

    pub(crate) async fn read_descriptors(&self) {
        let next = match descriptors::list(&self.local).await {
            Ok(all) => Loadable::Ready(all),
            Err(error) => {
                tracing::warn!(%error, "the descriptors could not be listed");
                Loadable::Failed
            },
        };
        *locked(&self.descriptors) = next;
        self.push(Change::Records(DESCRIPTORS.to_owned()));
    }

    /// Installs the YAML descriptor `text` as a new agent, then reloads the agents and the checks.
    pub fn upload_descriptor(
        self: &Arc<Self>,
        text: Vec<u8>,
    ) {
        let core = Arc::clone(self);
        self.handle.spawn(async move {
            core.install_descriptor(text).await;
        });
    }

    async fn install_descriptor(
        &self,
        text: Vec<u8>,
    ) {
        if let Err(error) = descriptors::upload(&self.local, text).await {
            (self.outcome)(Outcome::Failed(error.reason()));
        }
        self.read_agents().await;
        self.read_descriptors().await;
    }

    pub(crate) async fn read_permission(&self) {
        let answer = self.local.send(Call::get(PERMISSION_PATH)).await;
        let level = answer
            .inspect_err(|error| {
                tracing::warn!(%error, "the default permission level could not be read");
            })
            .ok()
            .and_then(|data| data.get("level")?.as_str().map(str::to_owned));
        let Some(level) = level else {
            return;
        };
        locked(&self.values).insert(PERMISSION.to_owned(), Value::Text(level.clone()));
        self.push(Change::Setting(Key::new(PERMISSION), Value::Text(level)));
    }

    pub(crate) async fn read_agents(&self) {
        let next = match providers::list(&self.local).await {
            Ok(all) => Loadable::Ready(all),
            Err(error) => {
                tracing::warn!(%error, "the agents could not be listed");
                Loadable::Failed
            },
        };
        *locked(&self.agents) = next;
        self.push(Change::Records(AGENTS.to_owned()));
    }
}

#[cfg(test)]
mod tests {
    use settings::Records;

    use super::*;
    use crate::test_support::{AGENTS, LEVEL_TRUSTED, SettingsRig};

    #[tokio::test]
    async fn the_agents_and_the_permission_level_come_from_the_daemon() {
        let rig = SettingsRig::open(&[LEVEL_TRUSTED, AGENTS]);
        assert_eq!(rig.core.records("agents"), Records::Loading);
        rig.agents_ready().await;
        assert!(matches!(rig.core.records("agents"), Records::Ready(list) if list.len() == 2));
        assert_eq!(
            rig.core.get(&Key::new("defaultPermissionLevel")),
            Some(Value::Text("trusted".into()))
        );
    }

    #[tokio::test]
    async fn the_lists_wait_for_the_link_and_fail_while_it_is_down_or_the_daemon_is_silent() {
        let waiting = SettingsRig::open_on(LinkState::Connecting, &[AGENTS]);
        tokio::task::yield_now().await;
        assert_eq!(waiting.core.records("agents"), Records::Loading);
        waiting
            .core
            .hub
            .edit(|state| state.link("local", LinkState::Live));
        waiting.agents_ready().await;
        assert!(matches!(waiting.core.records("agents"), Records::Ready(list) if list.len() == 2));
        let down = SettingsRig::open_on(LinkState::Down, &[AGENTS]);
        down.agents_ready().await;
        assert_eq!(down.core.records("agents"), Records::Failed);
        down.core
            .hub
            .edit(|state| state.link("local", LinkState::Live));
        down.agents_ready().await;
        assert!(matches!(down.core.records("agents"), Records::Ready(list) if list.len() == 2));
        let silent = SettingsRig::open(&[]);
        silent.agents_ready().await;
        assert_eq!(silent.core.records("agents"), Records::Failed);
    }
}
