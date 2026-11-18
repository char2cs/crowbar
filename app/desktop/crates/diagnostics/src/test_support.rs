//! In-memory stand-ins for the diagnostics ports.

use std::collections::BTreeMap;
use std::io;
use std::path::{Path, PathBuf};
use std::sync::{Mutex, PoisonError};

use crate::file_store::FileStore;

#[derive(Debug, Default)]
pub(crate) struct InMemoryFiles {
    files: Mutex<BTreeMap<PathBuf, Vec<u8>>>,
}

impl InMemoryFiles {
    pub(crate) fn with(
        self,
        path: &str,
        bytes: &[u8],
    ) -> Self {
        self.files
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .insert(PathBuf::from(path), bytes.to_vec());
        self
    }
}

impl FileStore for InMemoryFiles {
    fn read(
        &self,
        path: &Path,
    ) -> io::Result<Vec<u8>> {
        let files = self.files.lock().unwrap_or_else(PoisonError::into_inner);
        files
            .get(path)
            .cloned()
            .ok_or_else(|| io::Error::from(io::ErrorKind::NotFound))
    }

    fn list_files(
        &self,
        dir: &Path,
    ) -> io::Result<Vec<PathBuf>> {
        let files = self.files.lock().unwrap_or_else(PoisonError::into_inner);
        let listed: Vec<PathBuf> = files
            .keys()
            .filter(|path| path.parent() == Some(dir))
            .cloned()
            .collect();
        if listed.is_empty() {
            return Err(io::Error::from(io::ErrorKind::NotFound));
        }
        Ok(listed)
    }

    fn write(
        &self,
        path: &Path,
        bytes: &[u8],
    ) -> io::Result<()> {
        self.files
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .insert(path.to_path_buf(), bytes.to_vec());
        Ok(())
    }
}
