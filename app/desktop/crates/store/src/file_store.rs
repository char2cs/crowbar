//! The files of the machine the shell runs on, as the shell data layer sees them.

use std::io;
use std::path::{Path, PathBuf};

pub(crate) trait FileStore: std::fmt::Debug + Send + Sync {
    fn read(
        &self,
        path: &Path,
    ) -> io::Result<Vec<u8>>;

    fn write(
        &self,
        path: &Path,
        bytes: &[u8],
    ) -> io::Result<()>;

    fn children(
        &self,
        directory: &Path,
    ) -> Vec<PathBuf>;
}

pub(crate) fn read_text(
    files: &dyn FileStore,
    path: &Path,
) -> io::Result<Option<String>> {
    match files.read(path) {
        Ok(bytes) => String::from_utf8(bytes)
            .map(Some)
            .map_err(|error| io::Error::new(io::ErrorKind::InvalidData, error)),
        Err(source) if source.kind() == io::ErrorKind::NotFound => Ok(None),
        Err(source) => Err(source),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::FakeFiles;

    #[test]
    fn a_missing_file_reads_as_none_and_a_present_one_as_its_text() {
        let files = FakeFiles::default();
        files.put("/a.txt", b"hello");
        files.put("/bad", &[0xff, 0xfe]);
        assert_eq!(
            read_text(&files, Path::new("/a.txt")).unwrap().as_deref(),
            Some("hello")
        );
        assert_eq!(read_text(&files, Path::new("/none")).unwrap(), None);
        assert!(read_text(&files, Path::new("/bad")).is_err());
    }
}
