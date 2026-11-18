use std::collections::BTreeMap;
use std::io;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex};

use crate::file_store::FileStore;

/// A [`FileStore`] kept in memory; directories exist where a file lives below them.
#[derive(Debug, Default)]
pub(crate) struct FakeFiles {
    files: Mutex<BTreeMap<PathBuf, Vec<u8>>>,
    refuse_writes: Mutex<bool>,
}

fn held<T>(mutex: &Mutex<T>) -> std::sync::MutexGuard<'_, T> {
    mutex
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner)
}

/// Notes of paired hosts kept in memory.
pub(crate) fn notes() -> crate::host_notes::HostNotes {
    crate::host_notes::HostNotes::open_on(
        "/state/notes.json".into(),
        Arc::new(FakeFiles::default()),
    )
    .unwrap()
}

impl FakeFiles {
    pub(crate) fn put(
        &self,
        path: &str,
        bytes: &[u8],
    ) {
        held(&self.files).insert(PathBuf::from(path), bytes.to_vec());
    }

    pub(crate) fn text(
        &self,
        path: &Path,
    ) -> Option<String> {
        let bytes = held(&self.files).get(path).cloned()?;
        String::from_utf8(bytes).ok()
    }

    pub(crate) fn refuse_writes(&self) {
        *held(&self.refuse_writes) = true;
    }
}

impl FileStore for FakeFiles {
    fn read(
        &self,
        path: &Path,
    ) -> io::Result<Vec<u8>> {
        held(&self.files)
            .get(path)
            .cloned()
            .ok_or_else(|| io::Error::from(io::ErrorKind::NotFound))
    }

    fn write(
        &self,
        path: &Path,
        bytes: &[u8],
    ) -> io::Result<()> {
        if *held(&self.refuse_writes) {
            return Err(io::Error::from(io::ErrorKind::PermissionDenied));
        }
        held(&self.files).insert(path.to_path_buf(), bytes.to_vec());
        Ok(())
    }

    fn children(
        &self,
        directory: &Path,
    ) -> Vec<PathBuf> {
        let below = |path: &PathBuf| {
            let rest = path.strip_prefix(directory).ok()?;
            Some(directory.join(rest.components().next()?))
        };
        let mut found: Vec<PathBuf> = held(&self.files).keys().filter_map(below).collect();
        found.dedup();
        found
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn what_is_written_reads_back_and_directories_list_their_first_level() {
        let files = FakeFiles::default();
        files.write(Path::new("/a/b/c.txt"), b"x").unwrap();
        files.put("/a/d.txt", b"y");
        assert_eq!(files.read(Path::new("/a/d.txt")).unwrap(), b"y");
        assert_eq!(
            files.children(Path::new("/a")),
            [PathBuf::from("/a/b"), PathBuf::from("/a/d.txt")]
        );
        assert!(files.read(Path::new("/none")).is_err());
        files.refuse_writes();
        assert!(files.write(Path::new("/z"), b"").is_err());
    }
}
