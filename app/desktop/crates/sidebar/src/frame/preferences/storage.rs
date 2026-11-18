use std::io;
use std::path::Path;

pub(crate) trait Storage {
    fn path(&self) -> &Path;

    fn read(&self) -> io::Result<Option<String>>;

    fn write(
        &self,
        text: &str,
    ) -> io::Result<()>;
}
