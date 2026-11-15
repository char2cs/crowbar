use std::io::{self, Write};

use tracing_subscriber::fmt::MakeWriter;

use crate::log_sink::LogSink;

#[derive(Debug)]
pub(crate) struct SinkWriter {
    pub(crate) sink: LogSink,
}

impl Write for SinkWriter {
    fn write(
        &mut self,
        buf: &[u8],
    ) -> io::Result<usize> {
        let written = self.sink.file().write(buf)?;
        io::stderr().write_all(buf.get(..written).unwrap_or(buf))?;
        Ok(written)
    }

    fn flush(&mut self) -> io::Result<()> {
        self.sink.file().flush()
    }
}

impl MakeWriter<'_> for LogSink {
    type Writer = SinkWriter;

    fn make_writer(&self) -> SinkWriter {
        SinkWriter { sink: self.clone() }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rotating_log::RotatingLog;

    #[test]
    fn events_reach_the_file() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("crowbar.log");
        let sink = LogSink::new(RotatingLog::open(&path, 1024, 1).unwrap());
        let mut writer = sink.make_writer();
        writer.write_all(b"one event\n").unwrap();
        writer.flush().unwrap();
        assert_eq!(std::fs::read_to_string(&path).unwrap(), "one event\n");
    }
}
