//! The one-time move of the editor drafts that develop and the shell's own editor pages kept in
//! `IndexedDB` (decision D7 of the native editor plan).

mod idb;
mod script_value;

use std::path::{Path, PathBuf};

use crate::error::{Error, Result};
use crate::live::{DiskFiles, Files};

const MARKER: &str = "legacy-drafts-imported";
const APP_ID: &str = "software.rabbyte.crowbar";
const DATABASES: [&str; 3] = ["databases", "indexeddb", "v1"];
const DATABASE_FILE: &str = "IndexedDB.sqlite3";

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
            databases: databases(&DiskFiles::default(), (user_home, webview)),
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

fn databases(
    files: &dyn Files,
    (user_home, webview): (&Path, &Path),
) -> Vec<PathBuf> {
    let develop = user_home.join(".local/share").join(APP_ID);
    let pages = files.children(webview);
    std::iter::once(develop)
        .chain(pages)
        .flat_map(|root| under(files, &root))
        .collect()
}

fn under(
    files: &dyn Files,
    root: &Path,
) -> Vec<PathBuf> {
    let versions = DATABASES
        .iter()
        .fold(root.to_path_buf(), |path, part| path.join(part));
    files
        .children(&versions)
        .into_iter()
        .flat_map(|origin| files.children(&origin))
        .flat_map(|database| files.children(&database))
        .filter(|path| path.file_name().is_some_and(|name| name == DATABASE_FILE))
        .collect()
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

    #[test]
    fn develop_s_and_each_page_profile_s_databases_are_found() {
        let files = FakeFiles::default();
        let found = [
            "/home/u/.local/share/software.rabbyte.crowbar/databases/indexeddb/v1/tauri_localhost_0/AB/IndexedDB.sqlite3",
            "/state/webview/core/databases/indexeddb/v1/crowbar_local_0/AB/IndexedDB.sqlite3",
        ];
        let other =
            "/state/webview/core/databases/indexeddb/v1/crowbar_local_0/AB/IndexedDB.sqlite3-wal";
        for path in found.iter().chain([&other]) {
            files.write(Path::new(path), b"").unwrap();
        }
        let databases = databases(&files, (Path::new("/home/u"), Path::new("/state/webview")));
        let expected: Vec<PathBuf> = found.iter().map(PathBuf::from).collect();
        assert_eq!(databases, expected);
    }
}
