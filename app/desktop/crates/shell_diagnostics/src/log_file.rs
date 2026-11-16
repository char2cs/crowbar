//! An open log file: where to append and how much it already holds.

use std::io::Write;

pub(crate) struct LogFile {
    pub(crate) writer: Box<dyn Write + Send>,
    pub(crate) length: u64,
}
