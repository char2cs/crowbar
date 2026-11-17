use std::collections::BTreeMap;
use std::io::{Error, ErrorKind, Result};
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex, PoisonError};

use crate::byte_source::ByteSource;
use crate::layout_files::LayoutFiles;
use crate::layout_store::LayoutStore;
use crate::test_support::FixedClock;

pub(crate) const LAYOUT_PATH: &str = "/state/layout.json";

#[derive(Debug, Default)]
pub(crate) struct MemoryFiles {
    files: Mutex<BTreeMap<PathBuf, String>>,
    refuse_writes: bool,
}

impl MemoryFiles {
    pub(crate) fn refusing_writes() -> Self {
        Self {
            refuse_writes: true,
            ..Self::default()
        }
    }

    pub(crate) fn store(self: &Arc<Self>) -> LayoutStore {
        LayoutStore::new(
            PathBuf::from(LAYOUT_PATH),
            Arc::<Self>::clone(self),
            Arc::new(FixedClock::at(0)),
        )
    }

    pub(crate) fn seed(
        &self,
        path: &Path,
        text: &str,
    ) {
        self.lock().insert(path.to_path_buf(), text.to_owned());
    }

    pub(crate) fn text(
        &self,
        path: &Path,
    ) -> Option<String> {
        self.lock().get(path).cloned()
    }

    pub(crate) fn names(&self) -> Vec<PathBuf> {
        self.lock().keys().cloned().collect()
    }

    fn lock(&self) -> std::sync::MutexGuard<'_, BTreeMap<PathBuf, String>> {
        self.files.lock().unwrap_or_else(PoisonError::into_inner)
    }
}

impl LayoutFiles for MemoryFiles {
    fn read(
        &self,
        path: &Path,
    ) -> Result<String> {
        self.text(path)
            .ok_or_else(|| Error::from(ErrorKind::NotFound))
    }

    fn write(
        &self,
        path: &Path,
        bytes: &[u8],
    ) -> Result<()> {
        if self.refuse_writes {
            return Err(Error::from(ErrorKind::PermissionDenied));
        }
        self.seed(path, &String::from_utf8_lossy(bytes));
        Ok(())
    }

    fn rename(
        &self,
        from: &Path,
        to: &Path,
    ) -> Result<()> {
        let mut files = self.lock();
        let text = files
            .remove(from)
            .ok_or_else(|| Error::from(ErrorKind::NotFound))?;
        files.insert(to.to_path_buf(), text);
        Ok(())
    }
}

impl ByteSource for MemoryFiles {
    fn read_bytes(
        &self,
        path: &Path,
    ) -> Result<Vec<u8>> {
        self.read(path).map(String::into_bytes)
    }
}
