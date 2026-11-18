//! [`LogStore`] on the local disk.

use std::fs::OpenOptions;
use std::io;
use std::path::Path;

use crate::logging::log_file::LogFile;
use crate::logging::log_store::LogStore;

#[derive(Debug, Clone, Copy, Default)]
pub(crate) struct DiskLogStore(());

impl LogStore for DiskLogStore {
    fn open_append(
        &self,
        path: &Path,
    ) -> io::Result<LogFile> {
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent)?;
        }
        let file = OpenOptions::new().create(true).append(true).open(path)?;
        let length = file.metadata()?.len();
        Ok(LogFile {
            writer: Box::new(file),
            length,
        })
    }

    fn rename_over(
        &self,
        from: &Path,
        to: &Path,
    ) -> io::Result<()> {
        match std::fs::rename(from, to) {
            Err(error) if error.kind() == io::ErrorKind::NotFound => Ok(()),
            other => other,
        }
    }
}

#[cfg(test)]
mod tests {
    use std::io::Write;

    use super::*;

    #[test]
    #[ignore = "real-infra: the disk adapter of the LogStore port: append and rename on a real file"]
    fn appending_continues_and_renaming_a_missing_file_is_fine() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("logs/crowbar.log");
        let store = DiskLogStore::default();
        store
            .open_append(&path)
            .unwrap()
            .writer
            .write_all(b"abc")
            .unwrap();
        assert_eq!(store.open_append(&path).unwrap().length, 3);
        store
            .rename_over(&dir.path().join("missing"), &path)
            .unwrap();
        assert!(path.exists());
    }
}
