//! The record lists of the settings modal: what they hold and what can be done to a record.

use std::sync::Arc;

use daemonapi::Call;

use crate::live::{Outcome, OutcomeSink};
use settings::{Error, RecordCommand, Records};

use crate::store::settings_core::agents::{descriptor_records, records as agent_records};
use crate::store::settings_core::devices::device::{self, REVOKE_DEVICE};
use crate::store::settings_core::{AGENTS, DESCRIPTORS, HOSTS, SESSIONS, SettingsCore, locked};
use crate::store::settings_core::{
    live_records,
    live_records::{KILL, REMOVE_ANYWAY, REVOKE},
};

async fn end_session(
    client: daemonapi::Client,
    (path, title): (String, String),
    outcome: OutcomeSink,
) {
    let Err(error) = client.send(Call::post(path)).await else {
        return;
    };
    tracing::warn!(%error, "a session could not be ended");
    outcome(Outcome::Refused {
        title: format!("Couldn't kill the agent on {title}"),
        reason: crate::Error::Daemon(error).reason(),
    });
}

impl SettingsCore {
    pub(crate) fn records(
        &self,
        source: &str,
    ) -> Records {
        match source {
            AGENTS => locked(&self.agents).records(|all| agent_records::of(all)),
            SESSIONS => Records::Ready(self.hub.read(live_records::sessions)),
            HOSTS => Records::Ready(self.hub.read(live_records::hosts)),
            DESCRIPTORS => locked(&self.descriptors).records(|all| descriptor_records::of(all)),
            settings::DEVICE_RECORDS => locked(&self.devices).records(|all| device::records(all)),
            _ => Records::Failed,
        }
    }

    pub(crate) fn apply(
        self: &Arc<Self>,
        source: &str,
        record: &str,
        command: RecordCommand,
    ) -> Result<(), Error> {
        match (source, command) {
            (AGENTS, command) => self.edit_agents(record, command),
            (SESSIONS, RecordCommand::Run { action }) if action == KILL => self.kill(record),
            (HOSTS, RecordCommand::Run { action }) if action == REVOKE => {
                self.roster.revoke(&self.handle, record);
                Ok(())
            },
            (HOSTS, RecordCommand::Run { action }) if action == REMOVE_ANYWAY => {
                self.roster.remove_anyway(&self.handle, record);
                Ok(())
            },
            (settings::DEVICE_RECORDS, RecordCommand::Run { action })
                if action == REVOKE_DEVICE =>
            {
                self.revoke_device(record);
                Ok(())
            },
            (settings::DEVICE_RECORDS, RecordCommand::Refresh) => {
                self.reread_devices();
                Ok(())
            },
            (_, RecordCommand::Refresh) => {
                self.refresh();
                Ok(())
            },
            (source, _) => Err(Error::Unavailable(format!(
                "the {source} list cannot be changed from here"
            ))),
        }
    }

    fn kill(
        self: &Arc<Self>,
        chat: &str,
    ) -> Result<(), Error> {
        let client = self
            .hub
            .read(|state| state.client_of_chat(chat))
            .ok_or_else(|| Error::Unavailable("this session is not on a known host".to_owned()))?;
        let title = self
            .hub
            .read(|state| live_records::session_title(state, chat));
        let path = format!("/v0/chats/{chat}/kill");
        let outcome = Arc::clone(&self.outcome);
        self.handle
            .spawn(end_session(client, (path, title), outcome));
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::live::Outcome;
    use crate::test_support::{KILL, KILL_PATH, SettingsRig};

    fn kill() -> RecordCommand {
        RecordCommand::Run {
            action: "kill".into(),
        }
    }

    async fn until_told(rig: &SettingsRig) {
        while rig.told.lock().unwrap().is_empty() {
            tokio::task::yield_now().await;
        }
    }

    async fn first_told(rig: &SettingsRig) -> Option<Outcome> {
        let deadline = std::time::Duration::from_secs(5);
        tokio::time::timeout(deadline, until_told(rig)).await.ok()?;
        rig.told.lock().unwrap().first().cloned()
    }

    #[tokio::test]
    async fn a_session_is_killed_on_its_host_and_a_refusal_is_a_toast_under_develop_s_title() {
        let rig = SettingsRig::open(&[KILL]);
        rig.core.apply("sessions", "bo", kill()).unwrap();
        rig.called(KILL_PATH).await;
        let refused = SettingsRig::open(&[]);
        refused
            .daemon
            .answer(("POST", KILL_PATH), 500, r#"{"error":"rig fault"}"#);
        refused.core.apply("sessions", "bo", kill()).unwrap();
        let told = first_told(&refused).await;
        assert!(
            matches!(&told, Some(Outcome::Refused { title, .. }) if title == "Couldn't kill the agent on Idea"),
            "{told:?}"
        );
    }

    #[tokio::test]
    async fn unknown_sessions_are_refused_and_the_live_lists_are_served() {
        let rig = SettingsRig::open(&[]);
        assert!(rig.core.apply("sessions", "ghost", kill()).is_err());
        assert!(
            rig.core
                .apply("hosts", "local", RecordCommand::Refresh)
                .is_ok()
        );
        assert!(matches!(rig.core.records("sessions"), Records::Ready(list) if list.len() == 1));
        assert!(matches!(rig.core.records("hosts"), Records::Ready(list) if list.len() == 1));
    }
}
