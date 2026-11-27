use std::sync::Arc;

use daemonapi::{Call, Switch};
use settings::{Change, Error, RecordCommand};

use crate::live::Outcome;
use crate::live::Provider;
use crate::store::settings_core::agents::{
    records as agent_records,
    records::{ENABLED, TOOLS},
};
use crate::store::settings_core::loadable::Loadable;
use crate::store::settings_core::{AGENTS, SettingsCore, locked};

const PROVIDERS_PATH: &str = "/v0/settings/chat/providers";
const SAVE_FAILED: &str = "Could not save providers";
const UNREACHED: &str = "Crowbar could not reach the daemon \u{2014} try again.";

fn change_agent(
    all: &mut Vec<Provider>,
    at: usize,
    command: RecordCommand,
) -> Result<(), Error> {
    match command {
        RecordCommand::SetFlag { flag, on } if flag == ENABLED => {
            enable(all.get_mut(at), Some(on).into())
        },
        RecordCommand::SetFlag { flag, on } if flag == TOOLS => {
            enable_tools(all.get_mut(at), Some(on).into())
        },
        RecordCommand::SetFlag { .. } => Err(Error::Unavailable(
            "whether an agent is installed is detected by the daemon".to_owned(),
        )),
        RecordCommand::Move { to } => {
            let moved = all.remove(at);
            all.insert(to.min(all.len()), moved);
            Ok(())
        },
        _ => Err(Error::Unavailable("agents have no such action".to_owned())),
    }
}

fn enable(
    provider: Option<&mut Provider>,
    on: Switch,
) -> Result<(), Error> {
    edit_provider(provider, |held| held.enabled = on)
}

fn enable_tools(
    provider: Option<&mut Provider>,
    on: Switch,
) -> Result<(), Error> {
    edit_provider(provider, |held| held.mcp_enabled = on)
}

fn edit_provider(
    provider: Option<&mut Provider>,
    change: impl FnOnce(&mut Provider),
) -> Result<(), Error> {
    provider
        .map(change)
        .ok_or_else(|| Error::Unavailable("that agent is gone".to_owned()))
}

impl SettingsCore {
    pub(crate) fn edit_agents(
        self: &Arc<Self>,
        record: &str,
        command: RecordCommand,
    ) -> Result<(), Error> {
        let mut agents = locked(&self.agents);
        let Loadable::Ready(all) = &mut *agents else {
            return Err(Error::Unavailable("the agents are not loaded".to_owned()));
        };
        let before = all.clone();
        let at = all
            .iter()
            .position(|provider| provider.id == record)
            .ok_or_else(|| Error::Unavailable(format!("{record} is not an agent of this host")))?;
        change_agent(all, at, command)?;
        let body = agent_records::preferences(all);
        drop(agents);
        self.send_agents(body, before);
        Ok(())
    }

    fn send_agents(
        self: &Arc<Self>,
        body: serde_json::Value,
        before: Vec<Provider>,
    ) {
        let core = Arc::clone(self);
        self.handle
            .spawn(async move { core.put_agents(body, before).await });
    }

    async fn put_agents(
        &self,
        body: serde_json::Value,
        before: Vec<Provider>,
    ) {
        let saved = self
            .local
            .fetch::<Vec<Provider>>(Call::put(PROVIDERS_PATH).json(body))
            .await;
        let next = saved.unwrap_or_else(|error| self.save_refused(&error, before));
        *locked(&self.agents) = Loadable::Ready(next);
        self.push(Change::Records(AGENTS.to_owned()));
    }
}

impl SettingsCore {
    fn save_refused(
        &self,
        error: &daemonapi::Error,
        before: Vec<Provider>,
    ) -> Vec<Provider> {
        tracing::warn!(%error, "the agent preferences were not saved");
        (self.outcome)(Outcome::Refused {
            title: SAVE_FAILED.to_owned(),
            reason: UNREACHED.to_owned(),
        });
        before
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::test_support::{AGENTS, AGENTS_PUT, SettingsRig, agents_ready};
    use settings::Change;

    fn two_agent_updates(all: &[Change]) -> bool {
        all.iter()
            .filter(|change| agents_ready(std::slice::from_ref(change)))
            .count()
            >= 2
    }

    #[tokio::test]
    async fn agent_flags_and_order_are_put_as_the_whole_preference_list() {
        let rig = SettingsRig::open(&[AGENTS, AGENTS_PUT]);
        rig.agents_ready().await;
        let disable = RecordCommand::SetFlag {
            flag: "enabled".into(),
            on: false,
        };
        rig.core.apply("agents", "codex", disable).unwrap();
        rig.core
            .apply("agents", "codex", RecordCommand::Move { to: 0 })
            .unwrap();
        rig.changes_until(two_agent_updates).await;
        let puts: Vec<_> = rig
            .daemon
            .seen()
            .into_iter()
            .filter(|call| call.method == "PUT")
            .collect();
        assert_eq!(
            puts[0].body.clone().unwrap()["providers"][1],
            json!({"id": "codex", "disabled": true, "mcpDisabled": false})
        );
    }

    async fn until_told(rig: &SettingsRig) {
        while rig.told.lock().unwrap().is_empty() {
            tokio::task::yield_now().await;
        }
    }

    #[tokio::test]
    async fn an_installed_flag_and_an_unknown_agent_cannot_be_changed_and_a_refused_save_is_a_toast()
     {
        let rig = SettingsRig::open(&[AGENTS]);
        rig.agents_ready().await;
        let installed = RecordCommand::SetFlag {
            flag: "connected".into(),
            on: true,
        };
        assert!(rig.core.apply("agents", "codex", installed).is_err());
        assert!(
            rig.core
                .apply("agents", "ghost", RecordCommand::Refresh)
                .is_err()
        );
        let disable = RecordCommand::SetFlag {
            flag: "enabled".into(),
            on: false,
        };
        rig.core.apply("agents", "codex", disable).unwrap();
        let deadline = std::time::Duration::from_secs(5);
        let told = tokio::time::timeout(deadline, until_told(&rig)).await;
        assert!(told.is_ok(), "no toast for the refused save");
        let refused = Outcome::Refused {
            title: "Could not save providers".into(),
            reason: "Crowbar could not reach the daemon \u{2014} try again.".into(),
        };
        assert_eq!(*rig.told.lock().unwrap(), [refused]);
    }
}
