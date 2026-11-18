use std::io;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex, PoisonError};

use crate::hosts::hosts_storage::HostsStorage;

/// An in-memory hosts document that can be told to fail its next writes.
#[derive(Debug, Default)]
pub(crate) struct MemoryHostsStorage {
    path: PathBuf,
    bytes: Mutex<Option<Vec<u8>>>,
    failing: Mutex<bool>,
}

impl MemoryHostsStorage {
    pub(crate) fn shared() -> Arc<Self> {
        Arc::new(Self {
            path: PathBuf::from("hosts.json"),
            ..Self::default()
        })
    }

    pub(crate) fn fail_writes(&self) {
        *self.failing.lock().unwrap_or_else(PoisonError::into_inner) = true;
    }
}

impl HostsStorage for MemoryHostsStorage {
    fn location(&self) -> &Path {
        &self.path
    }

    fn load(&self) -> io::Result<Option<Vec<u8>>> {
        Ok(self
            .bytes
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .clone())
    }

    fn store(
        &self,
        bytes: &[u8],
    ) -> io::Result<()> {
        if *self.failing.lock().unwrap_or_else(PoisonError::into_inner) {
            return Err(io::Error::other("the disk refused the write"));
        }
        *self.bytes.lock().unwrap_or_else(PoisonError::into_inner) = Some(bytes.to_vec());
        Ok(())
    }
}
