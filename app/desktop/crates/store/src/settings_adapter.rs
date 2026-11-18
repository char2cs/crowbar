//! The settings modal's view of [`SettingsCore`]: the traits it needs, on the UI thread.

use std::cell::RefCell;
use std::collections::BTreeMap;
use std::rc::Rc;
use std::sync::Arc;

use settings::{
    Change, Error, Key, Listener, RecordBackend, RecordCommand, Records, SettingsFollow,
    SettingsRead, SettingsWrite, Snapshot, Subscription, Value,
};

use crate::settings_core::SettingsCore;

type Listeners = Rc<RefCell<BTreeMap<u64, Listener>>>;

/// Implements `SettingsBackend` and `RecordBackend` over the thread-safe core. Changes the core
/// made on other threads reach the modal's listeners when [`Self::deliver_pending`] runs.
#[derive(Clone)]
pub struct SettingsAdapter {
    core: Arc<SettingsCore>,
    listeners: Listeners,
    next: Rc<RefCell<u64>>,
}

impl std::fmt::Debug for SettingsAdapter {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("SettingsAdapter")
            .finish_non_exhaustive()
    }
}

impl SettingsAdapter {
    /// An adapter of `core`, to be used on the UI thread only.
    pub fn new(core: Arc<SettingsCore>) -> Self {
        Self {
            core,
            listeners: Rc::default(),
            next: Rc::default(),
        }
    }

    /// Installs the YAML agent descriptor `text`.
    pub fn upload_descriptor(
        &self,
        text: Vec<u8>,
    ) {
        self.core.upload_descriptor(text);
    }

    /// Tells the modal's listeners about every change made since the last call.
    pub fn deliver_pending(&self) {
        let listeners: Vec<Listener> = self.listeners.borrow().values().cloned().collect();
        for change in self.core.take_changes() {
            notify(&listeners, &change);
        }
    }
}

fn notify(
    listeners: &[Listener],
    change: &Change,
) {
    for listener in listeners {
        listener(change);
    }
}

impl SettingsRead for SettingsAdapter {
    fn snapshot(&self) -> Snapshot {
        self.core.snapshot()
    }

    fn get(
        &self,
        key: &Key,
    ) -> Option<Value> {
        self.core.get(key)
    }
}

impl SettingsWrite for SettingsAdapter {
    fn set(
        &self,
        key: &Key,
        value: Value,
    ) -> Result<(), Error> {
        self.core.set(key, value)
    }

    fn invoke(
        &self,
        id: &str,
    ) -> Result<(), Error> {
        self.core.invoke(id)
    }
}

impl SettingsFollow for SettingsAdapter {
    fn subscribe(
        &self,
        listener: Listener,
    ) -> Subscription {
        let id = {
            let mut next = self.next.borrow_mut();
            *next += 1;
            *next
        };
        self.listeners.borrow_mut().insert(id, listener);
        let listeners = Rc::clone(&self.listeners);
        Subscription::new(move || {
            listeners.borrow_mut().remove(&id);
        })
    }
}

impl RecordBackend for SettingsAdapter {
    fn records(
        &self,
        source: &str,
    ) -> Records {
        self.core.records(source)
    }

    fn apply(
        &self,
        source: &str,
        record: &str,
        command: RecordCommand,
    ) -> Result<(), Error> {
        self.core.apply(source, record, command)
    }
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;

    use super::*;
    use crate::change_signal::ChangeSignal;
    use crate::outcome::OutcomeSink;
    use crate::settings_wiring::SettingsWiring;
    use crate::test_support::{FakeFiles, state};

    fn quiet_signal() -> ChangeSignal {
        Arc::new(|| {})
    }

    fn quiet_outcome() -> OutcomeSink {
        Arc::new(|_| {})
    }

    #[tokio::test]
    async fn changes_the_core_made_reach_a_listener_until_it_is_dropped() {
        let (state, _) = state();
        let hub = crate::hub::Hub::new(state, quiet_signal());
        let local = hub.read(|state| state.hosts[0].client.clone());
        let file =
            crate::setting_file::SettingFile::new("/s.json".into(), Arc::new(FakeFiles::default()));
        let (fleet, desk) = crate::test_support::fleet_and_desk(&hub);
        let core = SettingsCore::open(SettingsWiring {
            file,
            hub: Arc::clone(&hub),
            handle: tokio::runtime::Handle::current(),
            local,
            signal: quiet_signal(),
            fonts: crate::system_fonts::SystemFonts::default(),
            outcome: quiet_outcome(),
            fleet,
            desk,
        })
        .unwrap();
        let adapter = SettingsAdapter::new(Arc::clone(&core));
        let heard = Rc::new(Cell::new(0));
        let counter = Rc::clone(&heard);
        let subscription = adapter.subscribe(Rc::new(move |_| counter.set(counter.get() + 1)));
        core.hub_changed();
        adapter.deliver_pending();
        assert_eq!(heard.get(), 2, "sessions and hosts");
        drop(subscription);
        core.hub_changed();
        adapter.deliver_pending();
        assert_eq!(heard.get(), 2);
        adapter
            .set(&Key::new("tabSize"), Value::Number(4.0))
            .unwrap();
        assert_eq!(adapter.get(&Key::new("tabSize")), Some(Value::Number(4.0)));
        assert!(adapter.invoke("hosts.add").is_ok());
    }
}
