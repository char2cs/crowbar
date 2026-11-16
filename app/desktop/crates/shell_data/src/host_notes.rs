//! The notes of every paired host, in one JSON file of the state directory.

use std::collections::BTreeMap;
use std::path::PathBuf;
use std::sync::{Mutex, MutexGuard, PoisonError};

use crate::error::{Error, Result};
use crate::host_note::HostNote;

/// The paired hosts' notes, loaded once and written through on every change.
#[derive(Debug)]
pub struct HostNotes {
    path: PathBuf,
    notes: Mutex<BTreeMap<String, HostNote>>,
}

impl HostNotes {
    /// The folder the notes file lives in.
    #[must_use]
    pub fn directory(&self) -> PathBuf {
        self.path.parent().map_or_else(PathBuf::new, PathBuf::from)
    }

    /// The notes stored at `path`; a missing file is no notes.
    ///
    /// # Errors
    /// [`Error::File`] or [`Error::Json`] when the file exists and cannot be read.
    pub fn open(path: PathBuf) -> Result<Self> {
        let notes = match std::fs::read_to_string(&path) {
            Ok(text) => serde_json::from_str(&text).map_err(|source| Error::Json {
                path: path.clone(),
                source,
            })?,
            Err(source) if source.kind() == std::io::ErrorKind::NotFound => BTreeMap::new(),
            Err(source) => return Err(Error::File { path, source }),
        };
        Ok(Self {
            path,
            notes: Mutex::new(notes),
        })
    }

    fn held(&self) -> MutexGuard<'_, BTreeMap<String, HostNote>> {
        self.notes.lock().unwrap_or_else(PoisonError::into_inner)
    }

    /// The note of `host`, when it has one.
    pub fn get(
        &self,
        host: &str,
    ) -> Option<HostNote> {
        self.held().get(host).cloned()
    }

    /// Every host that has a note, with it.
    pub fn all(&self) -> Vec<(String, HostNote)> {
        self.held()
            .iter()
            .map(|(host, note)| (host.clone(), note.clone()))
            .collect()
    }

    /// Keeps `note` for `host`.
    ///
    /// # Errors
    /// [`Error::File`] when the file cannot be written; the note is then not kept.
    pub fn put(
        &self,
        host: &str,
        note: HostNote,
    ) -> Result<()> {
        self.change(|notes| {
            notes.insert(host.to_owned(), note);
        })
    }

    /// Forgets the note of `host`.
    ///
    /// # Errors
    /// [`Error::File`] when the file cannot be written; the note is then still kept.
    pub fn remove(
        &self,
        host: &str,
    ) -> Result<()> {
        self.change(|notes| {
            notes.remove(host);
        })
    }

    fn change(
        &self,
        edit: impl FnOnce(&mut BTreeMap<String, HostNote>),
    ) -> Result<()> {
        let mut held = self.held();
        let mut next = held.clone();
        edit(&mut next);
        self.write(&next)?;
        *held = next;
        Ok(())
    }

    fn write(
        &self,
        notes: &BTreeMap<String, HostNote>,
    ) -> Result<()> {
        let text = serde_json::to_string_pretty(notes).map_err(|source| Error::Json {
            path: self.path.clone(),
            source,
        })?;
        atomic_file::write(&self.path, text.as_bytes()).map_err(|source| Error::File {
            path: self.path.clone(),
            source,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn note(name: &str) -> HostNote {
        HostNote {
            name: name.into(),
            client_id: "c".into(),
            fingerprint: "f".into(),
        }
    }

    #[test]
    fn notes_survive_a_reopen_and_forgetting_one_leaves_the_rest() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("state").join("paired.json");
        let notes = HostNotes::open(path.clone()).unwrap();
        assert_eq!(notes.all().len(), 0, "a missing file is no notes");
        notes.put("a", note("Alpha")).unwrap();
        notes.put("b", note("Beta")).unwrap();
        notes.remove("a").unwrap();
        let again = HostNotes::open(path).unwrap();
        assert_eq!(again.get("a"), None);
        assert_eq!(again.get("b"), Some(note("Beta")));
    }

    #[test]
    fn a_note_that_cannot_be_written_is_not_kept() {
        let dir = tempfile::tempdir().unwrap();
        let notes = HostNotes::open(dir.path().join("paired.json")).unwrap();
        std::fs::create_dir(dir.path().join("paired.json")).unwrap();
        assert!(notes.put("a", note("Alpha")).is_err());
        assert_eq!(notes.get("a"), None);
    }
}
