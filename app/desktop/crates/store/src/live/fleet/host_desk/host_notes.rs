use std::collections::BTreeMap;
use std::path::PathBuf;
use std::sync::{Arc, Mutex, MutexGuard, PoisonError};

use crate::error::{Error, Result};
use crate::live::files::DiskFiles;
use crate::live::files::{Files, read_text};
use crate::live::fleet::host_desk::note::Note;

#[derive(Debug)]
pub struct HostNotes {
    path: PathBuf,
    files: Arc<dyn Files>,
    notes: Mutex<BTreeMap<String, Note>>,
}

impl HostNotes {
    #[must_use]
    pub(crate) fn directory(&self) -> PathBuf {
        self.path.parent().map_or_else(PathBuf::new, PathBuf::from)
    }

    pub fn open(path: PathBuf) -> Result<Self> {
        Self::open_on(path, Arc::new(DiskFiles::default()))
    }

    pub(crate) fn open_on(
        path: PathBuf,
        files: Arc<dyn Files>,
    ) -> Result<Self> {
        let text = read_text(files.as_ref(), &path).map_err(|source| Error::File {
            path: path.clone(),
            source,
        })?;
        let notes = match text {
            Some(text) => serde_json::from_str(&text).map_err(|source| Error::Json {
                path: path.clone(),
                source,
            })?,
            None => BTreeMap::new(),
        };
        Ok(Self {
            path,
            files,
            notes: Mutex::new(notes),
        })
    }

    fn held(&self) -> MutexGuard<'_, BTreeMap<String, Note>> {
        self.notes.lock().unwrap_or_else(PoisonError::into_inner)
    }

    pub fn get(
        &self,
        host: &str,
    ) -> Option<Note> {
        self.held().get(host).cloned()
    }

    pub(crate) fn all(&self) -> Vec<(String, Note)> {
        self.held()
            .iter()
            .map(|(host, note)| (host.clone(), note.clone()))
            .collect()
    }

    pub(crate) fn put(
        &self,
        host: &str,
        note: Note,
    ) -> Result<()> {
        self.change(|notes| {
            notes.insert(host.to_owned(), note);
        })
    }

    pub(crate) fn remove(
        &self,
        host: &str,
    ) -> Result<()> {
        self.change(|notes| {
            notes.remove(host);
        })
    }

    fn change(
        &self,
        edit: impl FnOnce(&mut BTreeMap<String, Note>),
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
        notes: &BTreeMap<String, Note>,
    ) -> Result<()> {
        let text = serde_json::to_string_pretty(notes).map_err(|source| Error::Json {
            path: self.path.clone(),
            source,
        })?;
        self.files
            .write(&self.path, text.as_bytes())
            .map_err(|source| Error::File {
                path: self.path.clone(),
                source,
            })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::FakeFiles;

    fn note(name: &str) -> Note {
        Note {
            name: name.into(),
            client_id: "c".into(),
            fingerprint: "f".into(),
        }
    }

    fn open(files: &Arc<FakeFiles>) -> HostNotes {
        HostNotes::open_on("/state/paired.json".into(), Arc::<FakeFiles>::clone(files)).unwrap()
    }

    #[test]
    fn notes_survive_a_reopen_and_forgetting_one_leaves_the_rest() {
        let files = Arc::new(FakeFiles::default());
        let notes = open(&files);
        assert_eq!(notes.all().len(), 0, "a missing file is no notes");
        notes.put("a", note("Alpha")).unwrap();
        notes.put("b", note("Beta")).unwrap();
        notes.remove("a").unwrap();
        let again = open(&files);
        assert_eq!(again.get("a"), None);
        assert_eq!(again.get("b"), Some(note("Beta")));
    }

    #[test]
    fn notes_that_cannot_be_written_or_read_are_errors_not_empty_notes() {
        let files = Arc::new(FakeFiles::default());
        let notes = open(&files);
        files.refuse_writes();
        assert!(notes.put("a", note("Alpha")).is_err());
        assert_eq!(notes.get("a"), None);
        let files = Arc::new(FakeFiles::default());
        files.put("/state/paired.json", b"{");
        let opened = HostNotes::open_on("/state/paired.json".into(), files);
        assert!(matches!(opened, Err(Error::Json { .. })));
    }
}
