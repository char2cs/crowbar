use std::io;
use std::path::{Path, PathBuf};
use std::sync::{Mutex, MutexGuard, PoisonError};

use crate::state_file::StateFile;

/// A state file kept in memory.
#[derive(Debug)]
pub(crate) struct MemoryFile {
    path: PathBuf,
    text: Mutex<Option<String>>,
}

impl MemoryFile {
    pub(crate) fn with(text: Option<&str>) -> Self {
        Self {
            path: PathBuf::from("window.json"),
            text: Mutex::new(text.map(str::to_owned)),
        }
    }

    fn text(&self) -> MutexGuard<'_, Option<String>> {
        self.text.lock().unwrap_or_else(PoisonError::into_inner)
    }
}

impl StateFile for MemoryFile {
    fn path(&self) -> &Path {
        &self.path
    }

    fn read(&self) -> io::Result<Option<String>> {
        Ok(self.text().clone())
    }

    fn write(
        &self,
        text: &str,
    ) -> io::Result<()> {
        *self.text() = Some(text.to_owned());
        Ok(())
    }
}
