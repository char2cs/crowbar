use std::io;
use std::sync::{Arc, Mutex, PoisonError};

use crate::log_sink::LogSink;

/// A log sink that keeps its text in memory, shared with the test that reads it back.
#[derive(Debug, Clone, Default)]
pub(crate) struct MemorySink {
    text: Arc<Mutex<String>>,
}

impl MemorySink {
    pub(crate) fn text(&self) -> String {
        self.text
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .clone()
    }
}

impl LogSink for MemorySink {
    fn append(
        &mut self,
        text: &str,
    ) -> io::Result<()> {
        self.text
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .push_str(text);
        Ok(())
    }
}
