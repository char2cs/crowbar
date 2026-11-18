use std::fmt::Debug;
use std::io;
use std::path::Path;

pub(crate) trait HostsStorage: Debug + Send + Sync {
    fn location(&self) -> &Path;

    fn load(&self) -> io::Result<Option<Vec<u8>>>;

    fn store(
        &self,
        bytes: &[u8],
    ) -> io::Result<()>;
}
