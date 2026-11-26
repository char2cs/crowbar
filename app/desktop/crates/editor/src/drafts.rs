use std::collections::BTreeMap;

use crate::Draft;

mod adapters {
    pub(super) mod disk_draft_file;
}
pub(crate) mod draft;
mod draft_file;

pub use adapters::disk_draft_file::DiskDraftFile;
pub use draft_file::DraftFile;

/// The drafts of dirty documents, keyed by document, written through on each change, as
/// develop's `IndexedDB` `editor-drafts` store.
pub struct Drafts {
    file: Box<dyn DraftFile>,
    kept: BTreeMap<String, Draft>,
}

impl std::fmt::Debug for Drafts {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("Drafts")
            .field("kept", &self.kept.len())
            .finish_non_exhaustive()
    }
}

impl Drafts {
    /// The drafts `file` holds, and why none could be read when it cannot be read or holds no
    /// drafts: the drafts then start empty and the next draft kept replaces the file.
    pub fn open(file: Box<dyn DraftFile>) -> (Self, Option<std::io::Error>) {
        let (drafts, error) = match read_all(file.as_ref()) {
            Ok(drafts) => (drafts, None),
            Err(error) => (Vec::new(), Some(error)),
        };
        let kept = drafts
            .into_iter()
            .map(|draft| (draft.uri.clone(), draft))
            .collect();
        (Self { file, kept }, error)
    }

    /// The draft of `uri`, if one is kept.
    #[must_use]
    pub fn get(
        &self,
        uri: &str,
    ) -> Option<&Draft> {
        self.kept.get(uri)
    }

    /// Keeps `draft` in place of any earlier one of its document.
    ///
    /// # Errors
    /// When the drafts file cannot be written; the draft is still kept for this run.
    pub fn keep(
        &mut self,
        draft: Draft,
    ) -> std::io::Result<()> {
        if self.kept.get(&draft.uri) == Some(&draft) {
            return Ok(());
        }
        self.kept.insert(draft.uri.clone(), draft);
        self.write()
    }

    /// Keeps each of `drafts` whose document has no draft yet, the first of two for one document
    /// winning, and says how many were kept: drafts carried over from an older editor.
    ///
    /// # Errors
    /// When the drafts file cannot be written; the drafts are still kept for this run.
    pub fn adopt(
        &mut self,
        drafts: Vec<Draft>,
    ) -> std::io::Result<usize> {
        let before = self.kept.len();
        for draft in drafts {
            self.kept.entry(draft.uri.clone()).or_insert(draft);
        }
        let adopted = self.kept.len() - before;
        if adopted > 0 {
            self.write()?;
        }
        Ok(adopted)
    }

    /// Drops every draft whose document is not in `held`: the drafts of tabs that were not
    /// restored, as develop's `restoreDocuments` clears them.
    ///
    /// # Errors
    /// When the drafts file cannot be written; the drafts are still dropped for this run.
    pub fn keep_only(
        &mut self,
        held: &std::collections::BTreeSet<String>,
    ) -> std::io::Result<()> {
        let before = self.kept.len();
        self.kept.retain(|uri, _| held.contains(uri));
        if self.kept.len() == before {
            return Ok(());
        }
        self.write()
    }

    /// Drops the draft of `uri`: its document is clean or closed.
    ///
    /// # Errors
    /// When the drafts file cannot be written.
    pub fn discard(
        &mut self,
        uri: &str,
    ) -> std::io::Result<()> {
        if self.kept.remove(uri).is_none() {
            return Ok(());
        }
        self.write()
    }

    fn write(&self) -> std::io::Result<()> {
        let drafts: Vec<&Draft> = self.kept.values().collect();
        let text = serde_json::to_string(&drafts).map_err(std::io::Error::other)?;
        self.file.write(&text)
    }
}

fn read_all(file: &dyn DraftFile) -> std::io::Result<Vec<Draft>> {
    let Some(text) = file.read()? else {
        return Ok(Vec::new());
    };
    serde_json::from_str(&text).map_err(std::io::Error::other)
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::rc::Rc;

    use super::*;

    #[derive(Default, Clone)]
    struct MemoryFile(Rc<RefCell<Option<String>>>);

    impl DraftFile for MemoryFile {
        fn read(&self) -> std::io::Result<Option<String>> {
            Ok(self.0.borrow().clone())
        }

        fn write(
            &self,
            text: &str,
        ) -> std::io::Result<()> {
            *self.0.borrow_mut() = Some(text.to_owned());
            Ok(())
        }
    }

    fn draft(
        uri: &str,
        text: &str,
    ) -> Draft {
        Draft {
            uri: uri.to_owned(),
            text: text.to_owned(),
            saved_text: "saved".to_owned(),
        }
    }

    #[test]
    fn kept_and_adopted_drafts_survive_a_restart_and_a_discarded_one_does_not() {
        let file = MemoryFile::default();
        let (mut drafts, error) = Drafts::open(Box::new(file.clone()));
        assert!(error.is_none(), "no file yet");
        drafts.keep(draft("a", "one")).unwrap();
        drafts.keep(draft("a", "two")).unwrap();
        drafts.keep(draft("b", "three")).unwrap();
        drafts.discard("b").unwrap();
        let old = vec![draft("a", "old"), draft("c", "first"), draft("c", "second")];
        assert_eq!(drafts.adopt(old).unwrap(), 1, "only c is new");
        let (restarted, _) = Drafts::open(Box::new(file.clone()));
        assert_eq!(
            restarted.get("a"),
            Some(&draft("a", "two")),
            "kept beats adopted"
        );
        assert_eq!(restarted.get("b"), None);
        assert_eq!(
            restarted.get("c"),
            Some(&draft("c", "first")),
            "the first of a pair"
        );
        let mut restarted = restarted;
        restarted.keep_only(&["a".to_owned()].into()).unwrap();
        let (swept, _) = Drafts::open(Box::new(file));
        assert_eq!(swept.get("a"), Some(&draft("a", "two")), "its tab holds it");
        assert_eq!(swept.get("c"), None, "no tab holds it");
    }

    #[test]
    fn an_unreadable_file_starts_empty_and_says_why() {
        let file = MemoryFile(Rc::new(RefCell::new(Some("not json".to_owned()))));
        let (drafts, error) = Drafts::open(Box::new(file));
        assert!(error.is_some());
        assert_eq!(drafts.get("a"), None);
    }
}
