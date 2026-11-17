use std::sync::{Arc, Condvar, Mutex, MutexGuard, PoisonError};
use std::thread::JoinHandle;

use async_channel::{Receiver, Sender};
use model::Layout;

use crate::error::Error;
use crate::layout_store::LayoutStore;
use crate::pending_layout::PendingLayout;
use crate::save_health::SaveHealth;
use crate::save_report::SaveReport;

type Shared = Arc<(Mutex<PendingLayout>, Condvar)>;

#[derive(Debug)]
pub(crate) struct LayoutWriter {
    shared: Shared,
    thread: Option<JoinHandle<()>>,
    reports: Receiver<SaveReport>,
}

impl LayoutWriter {
    pub(crate) fn start(store: LayoutStore) -> std::io::Result<Self> {
        let shared = Shared::default();
        let worker = Arc::clone(&shared);
        let (announce, reports) = async_channel::unbounded();
        let thread = std::thread::Builder::new()
            .name("layout-writer".into())
            .spawn(move || run(&worker, &store, &announce))?;
        Ok(Self {
            shared,
            thread: Some(thread),
            reports,
        })
    }

    pub(crate) fn reports(&self) -> Receiver<SaveReport> {
        self.reports.clone()
    }

    pub(crate) fn submit(
        &self,
        layout: Layout,
    ) {
        lock(&self.shared).latest = Some(layout);
        self.shared.1.notify_one();
    }
}

impl Drop for LayoutWriter {
    fn drop(&mut self) {
        lock(&self.shared).closed = true;
        self.shared.1.notify_one();
        let Some(thread) = self.thread.take() else {
            return;
        };
        if thread.join().is_err() {
            tracing::error!("the layout writer panicked; the last layout may be unsaved");
        }
    }
}

fn lock(shared: &Shared) -> MutexGuard<'_, PendingLayout> {
    shared.0.lock().unwrap_or_else(PoisonError::into_inner)
}

fn run(
    shared: &Shared,
    store: &LayoutStore,
    announce: &Sender<SaveReport>,
) {
    let mut health = SaveHealth::default();
    while let Some(layout) = next(shared) {
        let outcome = save_logged(store, &layout);
        health
            .record(outcome)
            .into_iter()
            .for_each(|report| tell(announce, report));
    }
}

fn save_logged(
    store: &LayoutStore,
    layout: &Layout,
) -> Result<(), Error> {
    store.save(layout).inspect_err(|error| {
        tracing::error!(%error, "saving the layout failed");
    })
}

fn tell(
    announce: &Sender<SaveReport>,
    report: SaveReport,
) {
    if announce.try_send(report).is_err() {
        tracing::debug!("the shell stopped listening for save reports");
    }
}

fn nothing_to_do(pending: &PendingLayout) -> bool {
    pending.latest.is_none() && !pending.closed
}

fn next(shared: &Shared) -> Option<Layout> {
    let mut pending = lock(shared);
    while nothing_to_do(&pending) {
        pending = shared
            .1
            .wait(pending)
            .unwrap_or_else(PoisonError::into_inner);
    }
    pending.latest.take()
}

#[cfg(test)]
mod tests {
    use model::{Command, TabId};

    use super::*;

    fn add_tab(id: u64) -> Command {
        Command::AddTab {
            id: TabId(id),
            title: "t".into(),
        }
    }

    fn layout_with_tabs(count: u64) -> Layout {
        let mut layout = Layout::default();
        for id in 1..=count {
            layout.apply(add_tab(id)).unwrap();
        }
        layout
    }

    const CASES: &[u64] = &[1, 20];

    fn saved_after_submitting(newest: u64) -> Layout {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("layout.json");
        let writer = LayoutWriter::start(LayoutStore::new(path.clone())).unwrap();
        for count in 1..=newest {
            writer.submit(layout_with_tabs(count));
        }
        drop(writer);
        LayoutStore::new(path).load().unwrap()
    }

    #[test]
    fn dropping_the_writer_finishes_with_the_newest_layout() {
        for newest in CASES {
            let saved = saved_after_submitting(*newest);
            assert_eq!(saved, layout_with_tabs(*newest), "latest wins, none lost");
        }
    }

    #[test]
    fn a_writer_that_never_got_a_layout_writes_nothing() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("layout.json");
        drop(LayoutWriter::start(LayoutStore::new(path.clone())).unwrap());
        assert!(!path.exists(), "no file");
    }

    #[test]
    fn a_layout_that_cannot_be_written_is_reported_to_the_shell() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("layout.json");
        std::fs::create_dir(&path).unwrap();
        let writer = LayoutWriter::start(LayoutStore::new(path)).unwrap();
        let reports = writer.reports();
        writer.submit(layout_with_tabs(1));
        drop(writer);
        let report = reports.try_recv().unwrap();
        assert!(matches!(report, SaveReport::Failed(_)), "{report:?}");
        assert!(reports.try_recv().is_err(), "one report per streak");
    }
}
