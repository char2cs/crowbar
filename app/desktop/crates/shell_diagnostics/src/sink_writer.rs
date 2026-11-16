use std::io::{self, Write};

use tracing_subscriber::fmt::MakeWriter;

use crate::log_sink::LogSink;

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
    use std::path::Path;
    use std::sync::Arc;

    use super::*;
    use crate::log_store::LogStore;
    use crate::rotating_log::RotatingLog;
    use crate::test_support::in_memory_log_store::InMemoryLogStore;

    #[test]
    fn events_reach_the_file() {
        let store = Arc::new(InMemoryLogStore::default());
        let shared: Arc<dyn LogStore> = Arc::<InMemoryLogStore>::clone(&store);
        let path = Path::new("crowbar.log");
        let sink = LogSink::new(RotatingLog::open(shared, path, (1024, 1)).unwrap());
        let mut writer = sink.make_writer();
        writer.write_all(b"one event\n").unwrap();
        writer.flush().unwrap();
        assert_eq!(store.text(path).as_deref(), Some("one event\n"));
    }
}
