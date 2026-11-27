use std::sync::Arc;

use sidebar::RowId;

use crate::error::{Error, Result};
use crate::live::ChangeSignal;
use crate::live::State;
use crate::store::Store;
use crate::store::branch_import::BranchImport;
use crate::store::folder_browse::FolderBrowse;
use crate::store::folder_browse::{FolderChoice, folder_choice};
use crate::store::pairing::device_offer::DeviceOffer;
use crate::store::pairing::pairing_session::PairingSession;
use crate::store::settings_core::JsonFile;
use crate::store::settings_core::Parts;
use crate::store::settings_core::SystemFonts;
use crate::store::settings_core::{SettingsAdapter, SettingsCore};
use crate::store::system_check::SystemCheck;

const NO_LOCAL_HOST: &str = "no local host is followed";

impl Store {
    pub fn branch_import(
        &self,
        row: &RowId,
        signal: ChangeSignal,
    ) -> Result<Arc<BranchImport>> {
        let parts = (&self.hub, &self.commands.handle, &self.commands.outcome);
        BranchImport::open(parts, row.as_str(), signal)
    }

    pub fn folder_choice(&self) -> FolderChoice {
        self.hub.read(folder_choice::of)
    }

    pub fn folder_browse(
        &self,
        signal: ChangeSignal,
    ) -> Result<Arc<FolderBrowse>> {
        FolderBrowse::open((&self.hub, &self.commands.handle), signal)
    }

    pub fn folder_browse_on(
        &self,
        host: &str,
        signal: ChangeSignal,
    ) -> Result<Arc<FolderBrowse>> {
        FolderBrowse::open_on((&self.hub, &self.commands.handle), host, signal)
    }

    pub fn settings(
        &self,
        file: std::path::PathBuf,
        signal: ChangeSignal,
        fonts: SystemFonts,
    ) -> Result<SettingsAdapter> {
        let local = self
            .hub
            .read(State::local_client)
            .ok_or_else(|| Error::Stream(NO_LOCAL_HOST.to_owned()))?;
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

    pub fn system_check(
        &self,
        signal: ChangeSignal,
    ) -> Result<Arc<SystemCheck>> {
        let local = self
            .hub
            .read(State::local_client)
            .ok_or_else(|| Error::Stream(NO_LOCAL_HOST.to_owned()))?;
        Ok(SystemCheck::start(
            local,
            self.commands.handle.clone(),
            signal,
        ))
    }

    pub fn device_offer(
        &self,
        signal: ChangeSignal,
    ) -> Result<Arc<DeviceOffer>> {
        let local = self
            .hub
            .read(State::local_client)
            .ok_or_else(|| Error::Stream(NO_LOCAL_HOST.to_owned()))?;
        Ok(DeviceOffer::start(
            local,
            self.commands.handle.clone(),
            signal,
        ))
    }

    pub fn has_local_host(&self) -> bool {
        self.hub.read(|state| state.local_client().is_some())
    }

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
