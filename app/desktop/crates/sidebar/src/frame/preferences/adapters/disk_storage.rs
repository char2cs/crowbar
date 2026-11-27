use std::io::{self, ErrorKind};
use std::path::{Path, PathBuf};

use crate::frame::preferences::storage::Storage;

#[derive(Debug)]
pub(crate) struct DiskStorage {
    path: PathBuf,
}

impl DiskStorage {
    pub(crate) fn at(path: &Path) -> Self {
        Self {
            path: path.to_path_buf(),
        }
    }
}

impl Storage for DiskStorage {
    fn path(&self) -> &Path {
        &self.path
    }

    fn read(&self) -> io::Result<Option<String>> {
        match std::fs::read_to_string(&self.path) {
            Ok(text) => Ok(Some(text)),
            Err(error) if error.kind() == ErrorKind::NotFound => Ok(None),
            Err(error) => Err(error),
        }
    }

    fn write(
        &self,
        text: &str,
    ) -> io::Result<()> {
        atomicfile::write(&self.path, text.as_bytes())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    #[ignore = "real-infra: the disk adapter of the preferences port: a missing file and an atomic write"]
    fn a_missing_file_reads_as_nothing_and_a_written_one_comes_back() {
        let path = std::env::temp_dir().join(format!("sidebar-prefs-{}.json", std::process::id()));
        let file = DiskStorage::at(&path);
        assert_eq!(file.read().unwrap(), None, "first run");
        file.write("{}").unwrap();
        let read = file.read().unwrap();
        std::fs::remove_file(&path).unwrap();
        assert_eq!(read.as_deref(), Some("{}"), "saved text");
    }
}
