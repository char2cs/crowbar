//! Where the rotating log keeps its files.

use std::io;
use std::path::Path;

use crate::logging::log_file::LogFile;
pub(crate) trait LogStore: Send + Sync {
    fn open_append(
        &self,
        path: &Path,
    ) -> io::Result<LogFile>;
    fn rename_over(
        &self,
        from: &Path,
        to: &Path,
    ) -> io::Result<()>;
}
