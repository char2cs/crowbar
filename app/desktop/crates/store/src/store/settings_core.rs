//! Settings and record lists over the daemon and the shell's own settings file.

mod agents;
pub(crate) mod json_file;
mod lists;
mod live_records;
mod loadable;
pub(crate) mod parts;
mod reads;
mod settings_adapter;
mod system_fonts;

pub(crate) use json_file::JsonFile;
pub use json_file::LegacyImport;
pub(crate) use loadable::Loadable;
pub(crate) use parts::Parts;
pub use settings_adapter::SettingsAdapter;
pub use system_fonts::SystemFonts;

use std::collections::BTreeMap;
use std::sync::{Arc, Mutex, PoisonError};

use settings::{Change, Error, Key, Snapshot, Value, Values};
use tokio::runtime::Handle;

use crate::live::ChangeSignal;
use crate::live::Hub;
use crate::live::Provider;
use crate::live::Roster;
use crate::live::{Outcome, OutcomeSink};
use crate::store::settings_core::agents::descriptor_report::DescriptorReport;
use crate::store::settings_core::agents::permission_key::PERMISSION;
use crate::store::settings_core::system_fonts::choices as font_choices;

pub(crate) const AGENTS: &str = "agents";
pub(crate) const SESSIONS: &str = "sessions";
pub(crate) const HOSTS: &str = "hosts";
pub(crate) const DESCRIPTORS: &str = "descriptors";
const UPLOAD_DESCRIPTOR: &str = "agents.uploadDescriptor";
const ADD_HOST: &str = "hosts.add";
const CLEAR_BROWSING_DATA: &str = "browser.clearData";

pub(crate) struct SettingsCore {
    pub(crate) file: JsonFile,
    pub(crate) values: Mutex<BTreeMap<String, Value>>,
    pub(crate) agents: Mutex<Loadable<Vec<Provider>>>,
    pub(crate) descriptors: Mutex<Loadable<Vec<DescriptorReport>>>,
    outbox: Mutex<Vec<Change>>,
    pub(crate) hub: Arc<Hub>,
    pub(crate) handle: Handle,
    pub(crate) local: daemonapi::Client,
    signal: ChangeSignal,
    fonts: SystemFonts,
    pub(crate) outcome: OutcomeSink,
    pub(crate) roster: Arc<Roster>,
}

impl std::fmt::Debug for SettingsCore {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("SettingsCore")
            .finish_non_exhaustive()
    }
}

pub(crate) fn locked<T>(mutex: &Mutex<T>) -> std::sync::MutexGuard<'_, T> {
    mutex.lock().unwrap_or_else(PoisonError::into_inner)
}

impl SettingsCore {
    pub(crate) fn open(wiring: Parts) -> Result<Arc<Self>, crate::error::Error> {
        let values = wiring.file.load()?;
        let roster = Roster::new(
            (wiring.fleet, wiring.desk),
            (Arc::clone(&wiring.outcome), Arc::clone(&wiring.signal)),
        );
        let core = Arc::new(Self {
            file: wiring.file,
            values: Mutex::new(values),
            agents: Mutex::new(Loadable::Loading),
            descriptors: Mutex::new(Loadable::Loading),
            outbox: Mutex::new(Vec::new()),
            hub: wiring.hub,
            handle: wiring.handle,
            local: wiring.local,
            signal: wiring.signal,
            fonts: wiring.fonts,
            outcome: wiring.outcome,
            roster,
        });
        core.refresh();
        Ok(core)
    }

    pub(crate) fn refresh(self: &Arc<Self>) {
        let core = Arc::clone(self);
        self.handle.spawn(async move {
            core.when_the_daemon_answers().await;
            core.read_permission().await;
            core.read_agents().await;
            core.read_descriptors().await;
        });
    }

    pub(crate) fn push(
        &self,
        change: Change,
    ) {
        locked(&self.outbox).push(change);
        (self.signal)();
    }

    pub(crate) fn take_changes(&self) -> Vec<Change> {
        std::mem::take(&mut *locked(&self.outbox))
    }

    pub(crate) fn snapshot(&self) -> Snapshot {
        let mut values = Values::default();
        for (key, value) in locked(&self.values).iter() {
            values.set(Key::new(key.as_str()), value.clone());
        }
        Snapshot::new(values)
            .with_capability(CLEAR_BROWSING_DATA)
            .with_choices("themes", font_choices::themes())
            .with_choices("fonts", font_choices::fonts(&self.fonts))
            .with_choices("monoFonts", font_choices::monospace(&self.fonts))
            .with_choices("terminalFonts", font_choices::monospace(&self.fonts))
    }

    pub(crate) fn get(
        &self,
        key: &Key,
    ) -> Option<Value> {
        locked(&self.values).get(key.as_str()).cloned()
    }

    pub(crate) fn set(
        self: &Arc<Self>,
        key: &Key,
        value: Value,
    ) -> Result<(), Error> {
        let level = value.as_text().unwrap_or_default().to_owned();
        let announced = value.clone();
        let previous = locked(&self.values).insert(key.as_str().to_owned(), value);
        if key.as_str() == PERMISSION {
            self.send_permission(previous, level);
            return Ok(());
        }
        let saved = self.file.save(&self.local_values());
        if let Err(error) = saved {
            self.restore(key.as_str(), previous);
            return Err(Error::Unavailable(error.to_string()));
        }
        self.push(Change::Setting(key.clone(), announced));
        Ok(())
    }

    pub(crate) fn invoke(
        &self,
        id: &str,
    ) -> Result<(), Error> {
        let asked = match id {
            UPLOAD_DESCRIPTOR => Outcome::PickDescriptor,
            ADD_HOST => Outcome::PairHost,
            CLEAR_BROWSING_DATA => Outcome::ClearBrowsingData,
            other => return Err(Error::Unavailable(format!("{other} is not available here"))),
        };
        (self.outcome)(asked);
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::{AGENTS, LEVEL_GUARDED, SETTINGS_PATH, SettingsRig};

    #[tokio::test]
    async fn shell_settings_persist_to_the_file_and_are_announced() {
        let rig = SettingsRig::open(&[LEVEL_GUARDED, AGENTS]);
        rig.agents_ready().await;
        rig.core
            .set(&Key::new("theme"), Value::Text("dark".into()))
            .unwrap();
        let saved = rig.files.text(std::path::Path::new(SETTINGS_PATH)).unwrap();
        assert!(saved.contains("dark") && !saved.contains(PERMISSION));
        assert!(rig.announced("theme", "dark"));
        assert_eq!(
            rig.core.get(&Key::new("theme")),
            Some(Value::Text("dark".into()))
        );
    }

    #[tokio::test]
    async fn the_backend_actions_ask_the_shell_and_an_unknown_one_is_unavailable() {
        let rig = SettingsRig::open(&[]);
        assert!(rig.core.invoke("hosts.add").is_ok());
        assert!(rig.core.invoke("browser.clearData").is_ok());
        assert!(rig.core.invoke("hosts.bogus").is_err());
        assert_eq!(
            *locked(&rig.told),
            [Outcome::PairHost, Outcome::ClearBrowsingData]
        );
    }
}
