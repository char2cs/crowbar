use std::io::{self, ErrorKind};
use std::path::{Path, PathBuf};

use crate::window_store::state_file::StateFile;

#[derive(Debug)]
pub(crate) struct DiskFile {
    path: PathBuf,
}

impl DiskFile {
    pub(crate) fn at(path: &Path) -> Self {
        Self {
            path: path.to_path_buf(),
        }
    }
}

impl StateFile for DiskFile {
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
    #[ignore = "real-infra: the disk adapter of the window file port: nested directory created on first write"]
    fn a_missing_file_reads_as_nothing_and_a_written_one_comes_back_in_a_new_directory() {
        let dir = tempfile::tempdir().unwrap();
        let file = DiskFile::at(&dir.path().join("nested/window.json"));
        assert_eq!(file.read().unwrap(), None, "first run");
        file.write("{}").unwrap();
        assert_eq!(file.read().unwrap().as_deref(), Some("{}"), "saved text");
    }
}
