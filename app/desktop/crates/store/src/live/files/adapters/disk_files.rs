use std::io;
use std::path::{Path, PathBuf};

use crate::live::files::Files;

#[derive(Debug, Clone, Copy, Default)]
pub(crate) struct DiskFiles(());

impl Files for DiskFiles {
    fn read(
        &self,
        path: &Path,
    ) -> io::Result<Vec<u8>> {
        std::fs::read(path)
    }

    fn write(
        &self,
        path: &Path,
        bytes: &[u8],
    ) -> io::Result<()> {
        atomicfile::write(path, bytes)
    }

    fn children(
        &self,
        directory: &Path,
    ) -> Vec<PathBuf> {
        let Ok(read) = std::fs::read_dir(directory) else {
            return Vec::new();
        };
        let mut paths: Vec<PathBuf> = read.filter_map(|entry| Some(entry.ok()?.path())).collect();
        paths.sort();
        paths
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    #[ignore = "real-infra: the disk adapter of the Files port: atomic write and listing on a real directory"]
    fn a_written_file_reads_back_and_its_directory_lists_it() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("nested").join("a.txt");
        DiskFiles::default().write(&path, b"hi").unwrap();
        assert_eq!(DiskFiles::default().read(&path).unwrap(), b"hi");
        assert_eq!(
            DiskFiles::default().children(&dir.path().join("nested")),
            [path]
        );
        assert_eq!(
            DiskFiles::default().children(&dir.path().join("none")),
            Vec::<PathBuf>::new()
        );
    }
}
