use std::io;
use std::path::Path;

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
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    #[ignore = "real-infra: the disk adapter of the Files port: atomic write on a real directory"]
    fn a_written_file_reads_back() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("nested").join("a.txt");
        DiskFiles::default().write(&path, b"hi").unwrap();
        assert_eq!(DiskFiles::default().read(&path).unwrap(), b"hi");
    }
}
