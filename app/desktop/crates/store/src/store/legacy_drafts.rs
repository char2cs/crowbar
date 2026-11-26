//! The one-time move of the editor drafts that develop and the shell's own editor pages kept in
//! `IndexedDB` (decision D7 of the native editor plan).

mod idb;
mod locate;
mod script_value;

use std::path::{Path, PathBuf};

use crate::error::{Error, Result};
use crate::live::{DiskFiles, Files};

const MARKER: &str = "legacy-drafts-imported";

/// Where the old drafts may be, and the marker that says they were taken.
#[derive(Debug, Clone)]
pub struct LegacyDrafts {
    /// The `IndexedDB` databases found.
    pub databases: Vec<PathBuf>,
    /// The file whose presence says the drafts were already taken.
    pub marker: PathBuf,
}

impl LegacyDrafts {
    /// The drafts of a user whose home folder is `user_home`, for a shell whose pages keep their
    /// data under `webview` and whose own state lives in `state`.
    #[must_use]
    pub fn discover(
        user_home: &Path,
        (webview, state): (&Path, &Path),
    ) -> Self {
        Self {
            databases: locate::databases(&DiskFiles::default(), (user_home, webview)),
            marker: state.join(MARKER),
        }
    }

    /// Takes the drafts once, removing them from where they were: a later start finds the
    /// marker and takes nothing. A database that cannot be read is skipped and logged.
    ///
    /// # Errors
    /// [`Error::File`] when the marker cannot be written.
    pub fn take(&self) -> Result<Vec<editor::Draft>> {
        self.take_on(&DiskFiles::default())
    }

    fn take_on(
        &self,
        files: &dyn Files,
    ) -> Result<Vec<editor::Draft>> {
        if self.databases.is_empty() || files.read(&self.marker).is_ok() {
            return Ok(Vec::new());
        }
        let drafts = self
            .databases
            .iter()
            .flat_map(|database| drafts_in(database))
            .collect();
        files
            .write(&self.marker, b"")
            .map_err(|source| Error::File {
                path: self.marker.clone(),
                source,
            })?;
        Ok(drafts)
    }
}

fn drafts_in(database: &Path) -> Vec<editor::Draft> {
    idb::take_drafts(database).unwrap_or_else(|error| {
        tracing::warn!(%error, "the drafts of an old database were not imported");
        Vec::new()
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::FakeFiles;

    #[test]
    fn drafts_are_taken_once_and_a_home_without_old_databases_leaves_no_marker() {
        let files = FakeFiles::default();
        let marker = PathBuf::from("/state").join(MARKER);
        let none = LegacyDrafts {
            databases: Vec::new(),
            marker: marker.clone(),
        };
        assert_eq!(none.take_on(&files).unwrap(), []);
        assert!(files.read(&marker).is_err(), "nothing to mark");
        let unreadable = LegacyDrafts {
            databases: vec!["/nonexistent/IndexedDB.sqlite3".into()],
            marker: marker.clone(),
        };
        assert_eq!(unreadable.take_on(&files).unwrap(), []);
        assert!(
            files.read(&marker).is_ok(),
            "taken once, even if nothing was there"
        );
    }
}
