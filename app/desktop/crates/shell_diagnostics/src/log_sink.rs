use std::sync::{Arc, Mutex, MutexGuard, PoisonError};

use crate::rotating_log::RotatingLog;

#[derive(Debug, Clone)]
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

    use super::*;

    #[test]
    fn clones_write_into_the_same_file() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("crowbar.log");
        let sink = LogSink::new(RotatingLog::open(&path, 1024, 1).unwrap());
        let clone = sink.clone();
        clone.file().write_all(b"from the clone\n").unwrap();
        sink.file().flush().unwrap();
        assert_eq!(std::fs::read_to_string(&path).unwrap(), "from the clone\n");
    }
}
