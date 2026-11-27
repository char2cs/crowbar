use std::io;
use std::path::{Path, PathBuf};

use crate::file_store::FileStore;

#[derive(Debug, Clone, Copy, Default)]
pub(crate) struct DiskFiles(());

impl FileStore for DiskFiles {
    fn read(
        &self,
        path: &Path,
    ) -> io::Result<Vec<u8>> {
        std::fs::read(path)
    }

    fn list_files(
        &self,
        dir: &Path,
    ) -> io::Result<Vec<PathBuf>> {
        let mut files: Vec<PathBuf> = std::fs::read_dir(dir)?
            .flatten()
            .map(|item| item.path())
            .filter(|path| path.is_file())
            .collect();
        files.sort();
        Ok(files)
    }

    fn write(
        &self,
        path: &Path,
        bytes: &[u8],
    ) -> io::Result<()> {
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent)?;
        }
        std::fs::write(path, bytes)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    #[ignore = "real-infra: the disk adapter of the FileStore port: read, write and list on a real directory"]
    fn written_files_are_read_and_listed_in_order() {
        let dir = tempfile::tempdir().unwrap();
        let files = DiskFiles::default();
        for name in ["b.txt", "nested/a.txt"] {
            files
                .write(&dir.path().join(name), name.as_bytes())
                .unwrap();
        }
        let listed = files.list_files(dir.path()).unwrap();
        assert_eq!(listed, [dir.path().join("b.txt")]);
        let content = files.read(&dir.path().join("nested/a.txt")).unwrap();
        assert_eq!(content, b"nested/a.txt");
    }
}
