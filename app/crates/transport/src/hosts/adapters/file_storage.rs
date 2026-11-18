use std::fs::DirBuilder;
use std::io;
use std::os::unix::fs::DirBuilderExt;
use std::path::{Path, PathBuf};

use crate::hosts::storage::Storage;

const PRIVATE_DIR: u32 = 0o700;

#[derive(Debug)]
pub(crate) struct FileStorage {
    path: PathBuf,
}

impl FileStorage {
    pub(crate) fn new(path: PathBuf) -> Self {
        Self { path }
    }
}

impl Storage for FileStorage {
    fn location(&self) -> &Path {
        &self.path
    }

    fn load(&self) -> io::Result<Option<Vec<u8>>> {
        match std::fs::read(&self.path) {
            Ok(bytes) => Ok(Some(bytes)),
            Err(error) if error.kind() == io::ErrorKind::NotFound => Ok(None),
            Err(error) => Err(error),
        }
    }

    fn store(
        &self,
        bytes: &[u8],
    ) -> io::Result<()> {
        if let Some(dir) = self.path.parent() {
            DirBuilder::new()
                .recursive(true)
                .mode(PRIVATE_DIR)
                .create(dir)?;
        }
        atomicfile::write(&self.path, bytes)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_file_that_was_never_written_loads_as_nothing() {
        let storage = FileStorage::new(PathBuf::from("/nonexistent/crowbar/hosts.json"));
        assert!(storage.load().unwrap().is_none());
        assert_eq!(
            storage.location(),
            Path::new("/nonexistent/crowbar/hosts.json")
        );
    }

    #[test]
    fn a_directory_that_cannot_be_created_is_a_store_error() {
        let storage = FileStorage::new(PathBuf::from("/proc/crowbar/hosts.json"));
        assert!(storage.store(b"{}").is_err());
    }
}
