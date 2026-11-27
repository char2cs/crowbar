use std::io;
use std::sync::{Arc, Mutex, PoisonError};

use crate::supervisor::supervision::log::sink::Sink;

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

impl Sink for MemorySink {
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
