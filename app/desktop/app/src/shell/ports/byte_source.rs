use std::fmt::Debug;
use std::io;
use std::path::Path;

pub(crate) trait ByteSource: Debug + Send + Sync {
    fn read_bytes(
        &self,
        path: &Path,
    ) -> io::Result<Vec<u8>>;
}
