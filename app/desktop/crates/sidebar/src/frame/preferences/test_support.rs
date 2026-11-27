#![cfg(test)]

use std::cell::RefCell;
use std::io;
use std::path::{Path, PathBuf};

use crate::frame::preferences::storage::Storage;

#[derive(Debug)]
pub(crate) struct MemoryStorage {
    path: PathBuf,
    text: RefCell<Option<String>>,
}

impl MemoryStorage {
    pub(crate) fn with(text: Option<&str>) -> Self {
        Self {
            path: PathBuf::from("preferences.json"),
            text: RefCell::new(text.map(str::to_owned)),
        }
    }
}

impl Storage for MemoryStorage {
    fn path(&self) -> &Path {
        &self.path
    }

    fn read(&self) -> io::Result<Option<String>> {
        Ok(self.text.borrow().clone())
    }

    fn write(
        &self,
        text: &str,
    ) -> io::Result<()> {
        *self.text.borrow_mut() = Some(text.to_owned());
        Ok(())
    }
}
