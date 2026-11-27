use std::io::{self, Write};

pub(crate) struct ByteCounter(pub(crate) usize);

impl Write for ByteCounter {
    fn write(
        &mut self,
        buf: &[u8],
    ) -> io::Result<usize> {
        self.0 += buf.len();
        Ok(buf.len())
    }

    fn flush(&mut self) -> io::Result<()> {
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn counts_every_written_byte() {
        let mut counter = ByteCounter(0);
        counter.write_all(b"abc").unwrap();
        counter.write_all(b"de").unwrap();
        counter.flush().unwrap();
        assert_eq!(counter.0, 5, "bytes written");
    }
}
