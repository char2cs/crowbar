//! What the shell's dialogs and panels are handed by the data layer.

use std::sync::Arc;

use sidebar::RowId;

use crate::error::{Error, Result};
use crate::live::ChangeSignal;
use crate::live::State;
use crate::store::Store;
use crate::store::branch_import::BranchImport;
use crate::store::folder_browse::FolderBrowse;
use crate::store::folder_browse::{FolderChoice, folder_choice};
use crate::store::pairing::pairing_session::PairingSession;
use crate::store::settings_core::JsonFile;
use crate::store::settings_core::Parts;
use crate::store::settings_core::SystemFonts;
use crate::store::settings_core::{SettingsAdapter, SettingsCore};
use crate::store::system_check::SystemCheck;

impl Store {
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

    /// The settings the modal edits: the shell's file at `file`, and this computer's daemon,
    /// behind an adapter for the UI thread.
    ///
    /// # Errors
    /// [`Error::File`] or [`Error::Json`] when the settings file exists and cannot be read, and
    /// [`Error::Stream`] when no local host is followed.
    pub fn settings(
        &self,
        file: std::path::PathBuf,
        signal: ChangeSignal,
        fonts: SystemFonts,
    ) -> Result<SettingsAdapter> {
        let local = self
            .hub
            .read(State::local_client)
            .ok_or_else(|| Error::Stream("no local host is followed".to_owned()))?;
        let core = SettingsCore::open(Parts {
            file: JsonFile::new(file, Arc::clone(&self.commands.files)),
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
        Ok(SettingsAdapter::new(core))
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
        let spec = crate::live::HostSpec {
            id: "local".into(),
            name: "This computer".into(),
            endpoint: None,
        };
        let wiring = crate::store::ports::Ports {
            desk: desk(),
            signal: quiet(),
            outcome: Arc::new(|_| ()),
        };
        let data = Store::start(
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
