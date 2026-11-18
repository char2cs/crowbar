use std::io;
use std::path::Path;

use crate::byte_source::ByteSource;
use crate::layout_files::LayoutFiles;

#[derive(Debug, Clone, Copy, Default)]
pub(crate) struct DiskFiles(());

impl LayoutFiles for DiskFiles {
    fn read(
        &self,
        path: &Path,
    ) -> io::Result<String> {
        std::fs::read_to_string(path)
    }

    fn write(
        &self,
        path: &Path,
        bytes: &[u8],
    ) -> io::Result<()> {
        atomicfile::write(path, bytes)
    }

    fn rename(
        &self,
        from: &Path,
        to: &Path,
    ) -> io::Result<()> {
        std::fs::rename(from, to)
    }
}

impl ByteSource for DiskFiles {
    fn read_bytes(
        &self,
        path: &Path,
    ) -> io::Result<Vec<u8>> {
        std::fs::read(path)
    }
}

#[cfg(test)]
mod tests {
    use std::io::ErrorKind;

    use super::*;

    const MANIFEST: &str = concat!(env!("CARGO_MANIFEST_DIR"), "/Cargo.toml");

    #[test]
    fn reading_returns_the_text_and_a_missing_file_is_not_found() {
        let files = DiskFiles::default();
        assert!(
            files
                .read(Path::new(MANIFEST))
                .unwrap()
                .contains("crowbar_desktop")
        );
        assert!(
            !files.read_bytes(Path::new(MANIFEST)).unwrap().is_empty(),
            "bytes"
        );
        let missing = files.read(Path::new("/nonexistent-crowbar/layout.json"));
        assert_eq!(missing.unwrap_err().kind(), ErrorKind::NotFound);
    }

    #[test]
    fn writing_under_a_regular_file_and_renaming_a_missing_one_fail() {
        let files = DiskFiles::default();
        let beneath_a_file = Path::new(MANIFEST).join("layout.json");
        assert!(files.write(&beneath_a_file, b"{}").is_err(), "write");
        let missing = Path::new("/nonexistent-crowbar/a");
        let renamed = files.rename(missing, Path::new("/nonexistent-crowbar/b"));
        assert_eq!(renamed.unwrap_err().kind(), ErrorKind::NotFound);
    }
}
