use std::io::{self, Write};

pub(crate) struct ByteCounter(pub(crate) usize);

impl Write for ByteCounter {
    fn write(&mut self, buf: &[u8]) -> io::Result<usize> {
        self.0 += buf.len();
        Ok(buf.len())
    }

    fn flush(&mut self) -> io::Result<()> {
        Ok(())
    }
}

#[cfg(test)]
#[path = "byte_counter_tests.rs"]
mod tests;
