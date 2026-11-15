use std::sync::{Arc, Condvar, Mutex, MutexGuard, PoisonError};
use std::thread::JoinHandle;

use model::Layout;

use crate::pending_layout::PendingLayout;

use crate::layout_store::LayoutStore;

type Shared = Arc<(Mutex<PendingLayout>, Condvar)>;

#[derive(Debug)]
pub(crate) struct LayoutWriter {
    shared: Shared,
    thread: Option<JoinHandle<()>>,
}

impl LayoutWriter {
    pub(crate) fn start(store: LayoutStore) -> std::io::Result<Self> {
        let shared = Shared::default();
        let worker = Arc::clone(&shared);
        let thread = std::thread::Builder::new()
            .name("layout-writer".into())
            .spawn(move || run(&worker, &store))?;
        Ok(Self {
            shared,
            thread: Some(thread),
        })
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
) {
    while let Some(layout) = next(shared) {
        save_logged(store, &layout);
    }
}

fn save_logged(
    store: &LayoutStore,
    layout: &Layout,
) {
    if let Err(error) = store.save(layout) {
        tracing::error!(%error, "saving the layout failed");
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

    #[test]
    fn dropping_the_writer_finishes_with_the_newest_layout() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("layout.json");
        let writer = LayoutWriter::start(LayoutStore::new(path.clone())).unwrap();
        for count in 1..=20 {
            writer.submit(layout_with_tabs(count));
        }
        drop(writer);
        let saved = LayoutStore::new(path).load().unwrap();
        assert_eq!(
            saved,
            layout_with_tabs(20),
            "latest wins and nothing is lost on exit"
        );
    }

    #[test]
    fn a_writer_that_never_got_a_layout_writes_nothing() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("layout.json");
        drop(LayoutWriter::start(LayoutStore::new(path.clone())).unwrap());
        assert!(!path.exists(), "no file");
    }
}
