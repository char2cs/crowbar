//! The record lists of the settings modal: what they hold and what can be done to a record.

use std::sync::Arc;

use daemonapi::{Call, DaemonClient};
use settings::{Error, RecordCommand, Records};

use crate::shell_data::settings_core::agents::{descriptor_records, records as agent_records};
use crate::shell_data::settings_core::{
    AGENTS, DESCRIPTORS, HOSTS, SESSIONS, SettingsCore, locked,
};
use crate::shell_data::settings_core::{
    live_records,
    live_records::{KILL, REMOVE_ANYWAY, REVOKE},
};

async fn end_session(
    client: DaemonClient,
    path: String,
) {
    if let Err(error) = client.send(Call::post(path)).await {
        tracing::warn!(%error, "a session could not be ended");
    }
}

impl SettingsCore {
    /// The content of the list `source`.
    pub fn records(
        &self,
        source: &str,
    ) -> Records {
        match source {
            AGENTS => locked(&self.agents).records(|all| agent_records::of(all)),
            SESSIONS => Records::Ready(self.hub.read(live_records::sessions)),
            HOSTS => Records::Ready(self.hub.read(live_records::hosts)),
            DESCRIPTORS => locked(&self.descriptors).records(|all| descriptor_records::of(all)),
            _ => Records::Failed,
        }
    }

    /// Applies `command` to `record` of `source`.
    ///
    /// # Errors
    /// [`Error::Unavailable`] when the list is not editable or the owner is not loaded.
    pub fn apply(
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
        let path = format!("/v0/chats/{chat}/kill");
        self.handle.spawn(end_session(client, path));
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::{KILL, KILL_PATH, SettingsRig};

    fn kill() -> RecordCommand {
        RecordCommand::Run {
            action: "kill".into(),
        }
    }

    #[tokio::test]
    async fn a_session_is_killed_on_its_host() {
        let rig = SettingsRig::open(&[KILL]);
        rig.core.apply("sessions", "bo", kill()).unwrap();
        rig.called(KILL_PATH).await;
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
