use std::collections::BTreeMap;
use std::io;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Condvar, Mutex};

use crate::live::Files;

/// A [`Files`] kept in memory.
#[derive(Debug, Default)]
pub(crate) struct FakeFiles {
    files: Mutex<BTreeMap<PathBuf, Vec<u8>>>,
    refuse_writes: Mutex<bool>,
    hold: (Mutex<bool>, Condvar),
}

fn held<T>(mutex: &Mutex<T>) -> std::sync::MutexGuard<'_, T> {
    mutex
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner)
}

/// Notes of paired hosts kept in memory.
pub(crate) fn notes() -> crate::live::HostNotes {
    crate::live::HostNotes::open_on("/state/notes.json".into(), Arc::new(FakeFiles::default()))
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

    /// Makes every write wait, as a slow disk does, until [`Self::release_writes`].
    pub(crate) fn hold_writes(&self) {
        *held(&self.hold.0) = true;
    }

    /// Lets held writes finish.
    pub(crate) fn release_writes(&self) {
        *held(&self.hold.0) = false;
        self.hold.1.notify_all();
    }

    fn wait_while_held(&self) {
        let mut holding = held(&self.hold.0);
        while *holding {
            holding = self
                .hold
                .1
                .wait(holding)
                .unwrap_or_else(std::sync::PoisonError::into_inner);
        }
    }
}

impl Files for FakeFiles {
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
        self.wait_while_held();
        if *held(&self.refuse_writes) {
            return Err(io::Error::from(io::ErrorKind::PermissionDenied));
        }
        held(&self.files).insert(path.to_path_buf(), bytes.to_vec());
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn what_is_written_reads_back_until_writes_are_refused() {
        let files = FakeFiles::default();
        files.write(Path::new("/a/b/c.txt"), b"x").unwrap();
        files.put("/a/d.txt", b"y");
        assert_eq!(files.read(Path::new("/a/d.txt")).unwrap(), b"y");
        assert!(files.read(Path::new("/none")).is_err());
        files.refuse_writes();
        assert!(files.write(Path::new("/z"), b"").is_err());
    }
}
