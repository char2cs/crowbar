use std::sync::{Arc, Mutex, MutexGuard, PoisonError};

use crate::rotating_log::RotatingLog;

#[derive(Clone)]
pub(crate) struct LogSink {
    file: Arc<Mutex<RotatingLog>>,
}

impl LogSink {
    pub(crate) fn new(file: RotatingLog) -> Self {
        Self {
            file: Arc::new(Mutex::new(file)),
        }
    }

    pub(crate) fn file(&self) -> MutexGuard<'_, RotatingLog> {
        self.file.lock().unwrap_or_else(PoisonError::into_inner)
    }
}

#[cfg(test)]
mod tests {
    use std::io::Write;
    use std::path::Path;
    use std::sync::Arc;

    use super::*;
    use crate::test_support::in_memory_log_store::InMemoryLogStore;

    #[test]
    fn clones_write_into_the_same_file() {
        let store = Arc::new(InMemoryLogStore::default());
        let shared: Arc<dyn crate::log_store::LogStore> = Arc::<InMemoryLogStore>::clone(&store);
        let path = Path::new("crowbar.log");
        let sink = LogSink::new(RotatingLog::open(shared, path, (1024, 1)).unwrap());
        let clone = sink.clone();
        clone.file().write_all(b"from the clone\n").unwrap();
        sink.file().flush().unwrap();
        assert_eq!(store.text(path).as_deref(), Some("from the clone\n"));
    }
}
