use std::io::Write;

pub(crate) struct LogFile {
    pub(crate) writer: Box<dyn Write + Send>,
    pub(crate) length: u64,
}
