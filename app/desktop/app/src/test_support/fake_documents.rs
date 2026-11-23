use std::cell::RefCell;
use std::collections::BTreeMap;
use std::rc::Rc;

use protocol::DocumentPath;
use store::{FileDone, FileError};

use crate::document_source::DocumentSource;
use crate::editor_panes::EditorPanes;

type Answer = Result<String, u16>;

/// Documents served from memory: a known chat's file answers its text, or the refusal it was
/// given; an unknown chat is not loaded yet.
#[derive(Default)]
pub(crate) struct FakeDocuments {
    chats: RefCell<Vec<String>>,
    files: RefCell<BTreeMap<String, Answer>>,
    reads: RefCell<Vec<String>>,
    writes: RefCell<Vec<(String, String)>>,
}

impl FakeDocuments {
    pub(crate) fn loading(
        &self,
        chat: &str,
    ) {
        self.chats.borrow_mut().push(chat.to_owned());
    }

    pub(crate) fn holding(
        &self,
        path: &str,
        text: &str,
    ) {
        self.files
            .borrow_mut()
            .insert(path.to_owned(), Ok(text.to_owned()));
    }

    pub(crate) fn refusing(
        &self,
        path: &str,
        status: u16,
    ) {
        self.files.borrow_mut().insert(path.to_owned(), Err(status));
    }

    pub(crate) fn reads(&self) -> Vec<String> {
        self.reads.borrow().clone()
    }

    pub(crate) fn writes(&self) -> Vec<(String, String)> {
        self.writes.borrow().clone()
    }
}

impl DocumentSource for FakeDocuments {
    fn read(
        &self,
        chat: &str,
        file: &DocumentPath,
        done: FileDone<String>,
    ) -> Result<(), store::Error> {
        if !self.chats.borrow().iter().any(|known| known == chat) {
            return Err(store::Error::Stream(format!("chat {chat} is not loaded")));
        }
        self.reads.borrow_mut().push(file.as_str().to_owned());
        let answer = self
            .files
            .borrow()
            .get(file.as_str())
            .cloned()
            .unwrap_or(Err(404));
        done(answer.map_err(refused));
        Ok(())
    }

    fn write(
        &self,
        (_, file): (&str, &DocumentPath),
        text: &str,
        done: FileDone<()>,
    ) -> Result<(), store::Error> {
        let path = file.as_str().to_owned();
        self.writes
            .borrow_mut()
            .push((path.clone(), text.to_owned()));
        self.files.borrow_mut().insert(path, Ok(text.to_owned()));
        done(Ok(()));
        Ok(())
    }
}

fn refused(status: u16) -> FileError {
    FileError::Daemon(store::Error::Daemon(daemonapi::Error::Refused {
        path: "/v0/documents/content".into(),
        status,
        code: None,
        message: "refused".into(),
        data: Box::default(),
    }))
}

/// File panes whose chats never load, for tests that have no files to show.
pub(crate) fn idle_editors() -> EditorPanes {
    EditorPanes::new(
        Rc::new(FakeDocuments::default()),
        (Rc::new(|_| false), Rc::new(|_| None)),
        async_channel::unbounded().0,
    )
}
