//! What the shell's dialogs and panels are handed by the data layer.

use std::sync::Arc;

use sidebar::RowId;

use crate::branch_import::BranchImport;
use crate::change_signal::ChangeSignal;
use crate::error::{Error, Result};
use crate::folder_browse::FolderBrowse;
use crate::folder_choice::{self, FolderChoice};
use crate::pairing_session::PairingSession;
use crate::setting_file::SettingFile;
use crate::settings_core::SettingsCore;
use crate::settings_wiring::SettingsWiring;
use crate::shell_data::ShellData;
use crate::state::State;
use crate::system_check::SystemCheck;
use crate::system_fonts::SystemFonts;

impl ShellData {
    /// The import dialog's data for the repository header or branch `row`; the dialog loads the
    /// remote branches and calls `signal` as they arrive.
    ///
    /// # Errors
    /// [`Error::Stream`] when `row` is not a repository of a followed host.
    pub fn branch_import(
        &self,
        row: &RowId,
        signal: ChangeSignal,
    ) -> Result<Arc<BranchImport>> {
        let parts = (&self.hub, &self.commands.handle, &self.commands.outcome);
        BranchImport::open(parts, row.as_str(), signal)
    }

    /// How the folder of a new repository is chosen for the open space.
    pub fn folder_choice(&self) -> FolderChoice {
        self.hub.read(folder_choice::of)
    }

    /// The remote folder picker's data for the host of the open space; the picker lists
    /// directories as they are opened and calls `signal` as the answers arrive.
    ///
    /// # Errors
    /// [`Error::Stream`] when no space is open.
    pub fn folder_browse(
        &self,
        signal: ChangeSignal,
    ) -> Result<Arc<FolderBrowse>> {
        FolderBrowse::open((&self.hub, &self.commands.handle), signal)
    }

    /// The remote folder picker's data for the paired host `host`, for a space not yet made.
    ///
    /// # Errors
    /// [`Error::Stream`] when `host` is not followed.
    pub fn folder_browse_on(
        &self,
        host: &str,
        signal: ChangeSignal,
    ) -> Result<Arc<FolderBrowse>> {
        FolderBrowse::open_on((&self.hub, &self.commands.handle), host, signal)
    }

    /// The settings the modal edits: the shell's file at `file`, and this computer's daemon.
    ///
    /// # Errors
    /// [`Error::File`] or [`Error::Json`] when the settings file exists and cannot be read, and
    /// [`Error::Stream`] when no local host is followed.
    pub fn settings(
        &self,
        file: std::path::PathBuf,
        signal: ChangeSignal,
        fonts: SystemFonts,
    ) -> Result<Arc<SettingsCore>> {
        let local = self
            .hub
            .read(State::local_client)
            .ok_or_else(|| Error::Stream("no local host is followed".to_owned()))?;
        let core = SettingsCore::open(SettingsWiring {
            file: SettingFile::new(file, Arc::clone(&self.commands.files)),
            hub: Arc::clone(&self.hub),
            handle: self.commands.handle.clone(),
            local,
            signal,
            fonts,
            outcome: Arc::clone(&self.commands.outcome),
            fleet: Arc::clone(&self.fleet),
            desk: self.desk.clone(),
        })?;
        let reading = Arc::clone(&core);
        self.settings.install(move |key| reading.get(key));
        Ok(core)
    }

    /// Probes git and the forge CLIs on this computer's daemon, signalling `signal` as each answers.
    ///
    /// # Errors
    /// [`Error::Stream`] when no local host is followed.
    pub fn system_check(
        &self,
        signal: ChangeSignal,
    ) -> Result<Arc<SystemCheck>> {
        let local = self
            .hub
            .read(State::local_client)
            .ok_or_else(|| Error::Stream("no local host is followed".to_owned()))?;
        Ok(SystemCheck::start(
            local,
            self.commands.handle.clone(),
            signal,
        ))
    }

    /// Whether this install follows a daemon of its own; a phone does not.
    pub fn has_local_host(&self) -> bool {
        self.hub.read(|state| state.local_client().is_some())
    }

    /// A fresh attempt at pairing a host, signalling `signal` after each of its steps.
    pub fn pairing(
        &self,
        signal: ChangeSignal,
    ) -> Arc<PairingSession> {
        PairingSession::new((Arc::clone(&self.fleet), self.desk.clone()), signal)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::{FakeDaemon, desk, quiet};

    #[tokio::test]
    async fn the_dialogs_open_on_a_data_layer_that_follows_a_local_host() {
        let transport: Arc<dyn protocol::DaemonTransport> = FakeDaemon::shared();
        let spec = crate::host_spec::HostSpec {
            id: "local".into(),
            name: "This computer".into(),
            endpoint: None,
        };
        let wiring = crate::shell_wiring::ShellWiring {
            desk: desk(),
            signal: quiet(),
            outcome: Arc::new(|_| ()),
        };
        let data = ShellData::start(
            &tokio::runtime::Handle::current(),
            &transport,
            vec![spec],
            wiring,
        );
        assert!(data.has_local_host());
        assert_eq!(data.folder_choice(), FolderChoice::Native);
        assert!(data.system_check(quiet()).is_ok());
        assert!(data.pairing(quiet()).state().error.is_none());
        assert!(data.folder_browse_on("ghost", quiet()).is_err());
        assert!(data.branch_import(&RowId::from("ghost"), quiet()).is_err());
    }
}
