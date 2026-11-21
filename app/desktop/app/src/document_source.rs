use protocol::{DocumentPath, PaneId};
use store::{Documents, FileDone, FileError};

use crate::document_request::DocumentRequest;
use crate::request_queue::send;
use crate::saved_file::SavedFile;
use crate::shell_request::ShellRequest;

pub(crate) trait DocumentSource {
    /// Reads `file` of `chat`'s workspace (or of this computer, for an absolute path) and hands
    /// its text to `done`; an error now means `chat` is not loaded yet.
    fn read(
        &self,
        chat: &str,
        file: &DocumentPath,
        done: FileDone<String>,
    ) -> Result<(), store::Error>;

    /// Writes `text` as the whole content of `file` and hands the outcome to `done`.
    fn write(
        &self,
        target: (&str, &DocumentPath),
        text: &str,
        done: FileDone<()>,
    ) -> Result<(), store::Error>;
}

impl DocumentSource for Documents {
    fn read(
        &self,
        chat: &str,
        file: &DocumentPath,
        done: FileDone<String>,
    ) -> Result<(), store::Error> {
        Documents::read(self, chat, file, done)
    }

    fn write(
        &self,
        target: (&str, &DocumentPath),
        text: &str,
        done: FileDone<()>,
    ) -> Result<(), store::Error> {
        Documents::write(self, target, text, done)
    }
}

pub(crate) fn answer_into(
    requests: async_channel::Sender<ShellRequest>,
    pane: PaneId,
    file: DocumentPath,
) -> FileDone<String> {
    Box::new(move |result| {
        let read = result.map_err(|error| failure_of(&file, &error));
        send(
            &requests,
            ShellRequest::Document(DocumentRequest::Read { pane, read }),
        );
    })
}

pub(crate) fn saved_into(
    requests: async_channel::Sender<ShellRequest>,
    pane: PaneId,
    text: String,
) -> FileDone<()> {
    Box::new(move |result| {
        let request = match result {
            Ok(()) => ShellRequest::Document(DocumentRequest::Saved { pane, text }),
            Err(error) => save_failed(pane, &error),
        };
        send(&requests, request);
    })
}

fn failure_of(
    file: &DocumentPath,
    error: &FileError,
) -> editor::OpenFailure {
    let (status, code) = error.refusal();
    editor::OpenFailure::of(file.as_str(), status, code)
}

pub(crate) fn saved_as_into(
    requests: async_channel::Sender<ShellRequest>,
    pane: PaneId,
    saved: SavedFile,
) -> FileDone<()> {
    Box::new(move |result| {
        let request = match result {
            Ok(()) => ShellRequest::Document(DocumentRequest::SavedAs { pane, saved }),
            Err(error) => save_failed(pane, &error),
        };
        send(&requests, request);
    })
}

pub(crate) fn save_failed(
    pane: PaneId,
    error: &dyn std::fmt::Display,
) -> ShellRequest {
    let reason = error.to_string();
    ShellRequest::Document(DocumentRequest::SaveFailed { pane, reason })
}

#[cfg(test)]
mod tests {
    use crate::test_support::FakeDocuments;

    use super::*;

    const PATH: &str = "src/a.ts";

    #[test]
    fn a_read_reaches_the_shell_and_a_refusal_says_why_as_develop_does() {
        let documents = FakeDocuments::default();
        documents.loading("c1");
        documents.refusing(PATH, 413);
        let (requests, heard) = async_channel::unbounded();
        let (pane, file) = (PaneId::new("f1").unwrap(), DocumentPath::new(PATH).unwrap());
        let done = answer_into(requests, pane.clone(), file.clone());
        documents.read("c1", &file, done).unwrap();
        let refused = editor::OpenFailure::of(PATH, 413, None);
        let expected = ShellRequest::Document(DocumentRequest::Read {
            pane,
            read: Err(refused),
        });
        assert_eq!(heard.try_recv().unwrap(), expected);
    }
}
