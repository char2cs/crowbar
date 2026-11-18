//! Where diagnostics read and write whole files.

mod adapters;

pub(crate) use adapters::DiskFiles;

use std::io;
use std::path::{Path, PathBuf};
pub(crate) trait FileStore: std::fmt::Debug + Send + Sync {
    fn read(
        &self,
        path: &Path,
    ) -> io::Result<Vec<u8>>;
    fn list_files(
        &self,
        dir: &Path,
    ) -> io::Result<Vec<PathBuf>>;
    fn write(
        &self,
        path: &Path,
        bytes: &[u8],
    ) -> io::Result<()>;
}
