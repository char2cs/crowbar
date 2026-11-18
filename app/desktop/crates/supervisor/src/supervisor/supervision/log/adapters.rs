//! Writes the daemon log to disk and reads its tail.

mod log_writer;
mod tail;

pub(crate) use log_writer::LogWriter;
pub(crate) use tail::read_tail;
