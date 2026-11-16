//! A [`LogStore`] held in memory.

use std::collections::BTreeMap;
use std::io::{self, Write};
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex, PoisonError};

use crate::log_file::LogFile;
use crate::log_store::LogStore;

type Content = Arc<Mutex<Vec<u8>>>;

#[derive(Debug, Default)]
pub(crate) struct InMemoryLogStore {
    files: Mutex<BTreeMap<PathBuf, Content>>,
}

struct Appender(Content);

impl Write for Appender {
    fn write(
        &mut self,
        buf: &[u8],
    ) -> io::Result<usize> {
        self.0
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .extend_from_slice(buf);
        Ok(buf.len())
    }

    fn flush(&mut self) -> io::Result<()> {
        Ok(())
    }
}

impl InMemoryLogStore {
    pub(crate) fn text(
        &self,
        path: &Path,
    ) -> Option<String> {
        let files = self.files.lock().unwrap_or_else(PoisonError::into_inner);
        let content = files.get(path)?;
        let bytes = content.lock().unwrap_or_else(PoisonError::into_inner);
        Some(String::from_utf8_lossy(&bytes).into_owned())
    }
}

impl LogStore for InMemoryLogStore {
    fn open_append(
        &self,
        path: &Path,
    ) -> io::Result<LogFile> {
        let mut files = self.files.lock().unwrap_or_else(PoisonError::into_inner);
        let content = Arc::clone(files.entry(path.to_path_buf()).or_default());
        let length = content.lock().unwrap_or_else(PoisonError::into_inner).len();
        Ok(LogFile {
            writer: Box::new(Appender(content)),
            length: u64::try_from(length).unwrap_or(u64::MAX),
        })
    }

    fn rename_over(
        &self,
        from: &Path,
        to: &Path,
    ) -> io::Result<()> {
        let mut files = self.files.lock().unwrap_or_else(PoisonError::into_inner);
        if let Some(content) = files.remove(from) {
            files.insert(to.to_path_buf(), content);
        }
        Ok(())
    }
}
