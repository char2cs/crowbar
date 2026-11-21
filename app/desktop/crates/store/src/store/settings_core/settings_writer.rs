//! Saves the settings file on its own thread, so the disk never holds up the window.

mod pending_save;

use std::collections::BTreeMap;
use std::sync::{Arc, Condvar, Mutex, MutexGuard, PoisonError};
use std::thread::JoinHandle;

use settings::Value;

use crate::live::{Outcome, OutcomeSink};
use crate::store::settings_core::json_file::JsonFile;
use crate::store::settings_core::settings_writer::pending_save::PendingSave;

type Shared = Arc<(Mutex<PendingSave>, Condvar)>;

#[derive(Debug)]
pub(crate) struct SettingsWriter {
    shared: Shared,
    thread: Option<JoinHandle<()>>,
}

impl SettingsWriter {
    pub(crate) fn start(
        file: JsonFile,
        outcome: OutcomeSink,
    ) -> std::io::Result<Self> {
        let shared = Shared::default();
        let worker = Arc::clone(&shared);
        let thread = std::thread::Builder::new()
            .name("settings-writer".into())
            .spawn(move || run(&worker, &file, &outcome))?;
        Ok(Self {
            shared,
            thread: Some(thread),
        })
    }

    pub(crate) fn submit(
        &self,
        values: BTreeMap<String, Value>,
    ) {
        lock(&self.shared).latest = Some(values);
        self.shared.1.notify_all();
    }

    pub(crate) fn flush(&self) {
        let mut pending = lock(&self.shared);
        while !idle(&pending) {
            pending = self
                .shared
                .1
                .wait(pending)
                .unwrap_or_else(PoisonError::into_inner);
        }
    }
}

impl Drop for SettingsWriter {
    fn drop(&mut self) {
        lock(&self.shared).closed = true;
        self.shared.1.notify_all();
        let Some(thread) = self.thread.take() else {
            return;
        };
        if thread.join().is_err() {
            tracing::error!("the settings writer panicked; the last settings may be unsaved");
        }
    }
}

fn lock(shared: &Shared) -> MutexGuard<'_, PendingSave> {
    shared.0.lock().unwrap_or_else(PoisonError::into_inner)
}

fn run(
    shared: &Shared,
    file: &JsonFile,
    outcome: &OutcomeSink,
) {
    while let Some(values) = next(shared) {
        save(file, outcome, &values);
        lock(shared).writing = false;
        shared.1.notify_all();
    }
}

fn save(
    file: &JsonFile,
    outcome: &OutcomeSink,
    values: &BTreeMap<String, Value>,
) {
    let Err(error) = file.save(values) else {
        return;
    };
    tracing::error!(%error, "saving the settings failed");
    outcome(Outcome::Failed(format!(
        "The settings were not saved: {}",
        error.reason()
    )));
}

fn idle(pending: &PendingSave) -> bool {
    pending.latest.is_none() && !pending.writing
}

fn waiting(pending: &PendingSave) -> bool {
    pending.latest.is_none() && !pending.closed
}

fn next(shared: &Shared) -> Option<BTreeMap<String, Value>> {
    let mut pending = lock(shared);
    while waiting(&pending) {
        pending = shared
            .1
            .wait(pending)
            .unwrap_or_else(PoisonError::into_inner);
    }
    let latest = pending.latest.take()?;
    pending.writing = true;
    Some(latest)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::FakeFiles;

    const PATH: &str = "/state/settings.json";

    type Told = Arc<Mutex<Vec<Outcome>>>;

    fn writer_on(files: &Arc<FakeFiles>) -> (SettingsWriter, JsonFile, Told) {
        let file = JsonFile::new(PATH.into(), Arc::<FakeFiles>::clone(files));
        let told = Arc::new(Mutex::new(Vec::new()));
        let heard = Arc::clone(&told);
        let outcome: OutcomeSink = Arc::new(move |said| {
            heard
                .lock()
                .unwrap_or_else(PoisonError::into_inner)
                .push(said);
        });
        let writer = SettingsWriter::start(file.clone(), outcome).unwrap();
        (writer, file, told)
    }

    fn sized(size: f64) -> BTreeMap<String, Value> {
        BTreeMap::from([("uiFontSize".to_owned(), Value::Number(size))])
    }

    #[test]
    fn dropping_the_writer_finishes_with_the_newest_values() {
        let files = Arc::new(FakeFiles::default());
        let (writer, file, _) = writer_on(&files);
        files.hold_writes();
        writer.submit(sized(12.0));
        writer.submit(sized(14.0));
        writer.submit(sized(16.0));
        files.release_writes();
        drop(writer);
        let saved = file.load().unwrap();
        assert_eq!(saved.get("uiFontSize"), Some(&Value::Number(16.0)));
    }

    #[test]
    fn a_save_the_disk_refuses_is_reported_for_a_toast() {
        let files = Arc::new(FakeFiles::default());
        let (writer, _, told) = writer_on(&files);
        files.refuse_writes();
        writer.submit(sized(12.0));
        writer.flush();
        let told = told.lock().unwrap_or_else(PoisonError::into_inner).clone();
        assert!(
            matches!(told.as_slice(), [Outcome::Failed(reason)] if reason.starts_with("The settings were not saved")),
            "{told:?}"
        );
    }
}
