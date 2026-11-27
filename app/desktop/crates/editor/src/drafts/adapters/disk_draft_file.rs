use std::io::ErrorKind;
use std::path::PathBuf;

use crate::drafts::DraftFile;

#[derive(Debug)]
pub struct DiskDraftFile {
    path: PathBuf,
}

impl DiskDraftFile {
    #[must_use]
    pub const fn at(path: PathBuf) -> Self {
        Self { path }
    }
}

impl DraftFile for DiskDraftFile {
    fn read(&self) -> std::io::Result<Option<String>> {
        match std::fs::read_to_string(&self.path) {
            Ok(text) => Ok(Some(text)),
            Err(error) if error.kind() == ErrorKind::NotFound => Ok(None),
            Err(error) => Err(error),
        }
    }

    fn write(
        &self,
        text: &str,
    ) -> std::io::Result<()> {
        atomicfile::write(&self.path, text.as_bytes())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    #[ignore = "real-infra: the disk adapter of the drafts file port: missing file, then a written one"]
    fn a_missing_file_reads_as_nothing_and_a_written_one_comes_back() {
        let dir = tempfile::tempdir().unwrap();
        let file = DiskDraftFile::at(dir.path().join("nested/editor-drafts.json"));
        assert_eq!(file.read().unwrap(), None, "first run");
        file.write("[]").unwrap();
        assert_eq!(file.read().unwrap().as_deref(), Some("[]"));
    }
}
