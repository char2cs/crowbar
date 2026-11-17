use std::fmt::Debug;
use std::io;
use std::path::Path;

pub(crate) trait LayoutFiles: Debug + Send + Sync {
    fn read(
        &self,
        path: &Path,
    ) -> io::Result<String>;
    fn write(
        &self,
        path: &Path,
        bytes: &[u8],
    ) -> io::Result<()>;
    fn rename(
        &self,
        from: &Path,
        to: &Path,
    ) -> io::Result<()>;
}
