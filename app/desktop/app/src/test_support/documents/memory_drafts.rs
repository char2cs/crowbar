use std::cell::RefCell;
use std::rc::Rc;

/// A drafts file kept in memory and shared, so a test can start the file panes again over it.
#[derive(Debug, Default, Clone)]
pub(crate) struct MemoryDraftFile(Rc<RefCell<Option<String>>>);

impl editor::DraftFile for MemoryDraftFile {
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

/// The drafts `file` holds.
pub(crate) fn memory_drafts(file: &MemoryDraftFile) -> editor::Drafts {
    editor::Drafts::open(Box::new(file.clone())).0
}
