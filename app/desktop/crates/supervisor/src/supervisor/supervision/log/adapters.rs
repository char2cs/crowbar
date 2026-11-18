//! Writes the daemon log to disk and reads its tail.

mod tail;
mod writer;

pub(crate) use tail::read_tail;
pub(crate) use writer::Writer;
