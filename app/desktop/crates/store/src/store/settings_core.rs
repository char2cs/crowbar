//! Settings and record lists over the daemon and the shell's own settings file.

mod agents;
pub(crate) mod json_file;
mod lists;
mod live_records;
mod loadable;
pub(crate) mod parts;
mod reads;
mod settings_adapter;
mod settings_writer;
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
use crate::store::settings_core::settings_writer::SettingsWriter;
use crate::store::settings_core::system_fonts::choices as font_choices;

pub(crate) const AGENTS: &str = "agents";
pub(crate) const SESSIONS: &str = "sessions";
pub(crate) const HOSTS: &str = "hosts";
pub(crate) const DESCRIPTORS: &str = "descriptors";
const UPLOAD_DESCRIPTOR: &str = "agents.uploadDescriptor";
const ADD_HOST: &str = "hosts.add";
const CLEAR_BROWSING_DATA: &str = "browser.clearData";

pub(crate) struct SettingsCore {
    writer: SettingsWriter,
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
        let writer = SettingsWriter::start(wiring.file, Arc::clone(&wiring.outcome))
            .map_err(crate::error::Error::Writer)?;
        let roster = Roster::new(
            (wiring.fleet, wiring.desk),
            (Arc::clone(&wiring.outcome), Arc::clone(&wiring.signal)),
        );
        let core = Arc::new(Self {
            writer,
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
    ) {
        let level = value.as_text().unwrap_or_default().to_owned();
        let announced = value.clone();
        let previous = self.store(key, value);
        if key.as_str() == PERMISSION {
            return self.send_permission(previous, level);
        }
        self.push(Change::Setting(key.clone(), announced));
    }

    fn store(
        &self,
        key: &Key,
        value: Value,
    ) -> Option<Value> {
        let mut values = locked(&self.values);
        let previous = values.insert(key.as_str().to_owned(), value);
        if key.as_str() != PERMISSION {
            let mut local = values.clone();
            local.remove(PERMISSION);
            self.writer.submit(local);
        }
        previous
    }

    pub(crate) fn flush(&self) {
        self.writer.flush();
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

    const DEADLINE: std::time::Duration = std::time::Duration::from_secs(5);

    fn set_themes_on_a_thread(core: &Arc<SettingsCore>) -> std::sync::mpsc::Receiver<()> {
        let core = Arc::clone(core);
        let (done, finished) = std::sync::mpsc::channel();
        std::thread::spawn(move || {
            core.set(&Key::new("theme"), Value::Text("dark".into()));
            core.set(&Key::new("theme"), Value::Text("light".into()));
            done.send(()).unwrap_or_default();
        });
        finished
    }

    #[tokio::test]
    async fn a_setting_is_read_and_announced_at_once_while_the_disk_is_slow_and_the_last_one_is_saved()
     {
        let rig = SettingsRig::open(&[LEVEL_GUARDED, AGENTS]);
        rig.agents_ready().await;
        rig.files.hold_writes();
        let returned = set_themes_on_a_thread(&rig.core).recv_timeout(DEADLINE);
        let read = rig.core.get(&Key::new("theme"));
        rig.files.release_writes();
        assert!(returned.is_ok(), "setting a value waited on the disk");
        assert_eq!(read, Some(Value::Text("light".into())));
        assert!(rig.announced("theme", "light"));
        rig.core.flush();
        let saved = rig.files.text(std::path::Path::new(SETTINGS_PATH)).unwrap();
        assert!(
            saved.contains("light") && !saved.contains("dark") && !saved.contains(PERMISSION),
            "{saved}"
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
