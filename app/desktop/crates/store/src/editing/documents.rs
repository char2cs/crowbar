//! The documents editor panes show, read from the daemon that holds them.

const CONTENT: &str = "/v0/documents/content";
const BASE64: &str = "base64";

use std::sync::Arc;

use base64::Engine as _;
use protocol::DocumentPath;
use serde_json::{Value, json};
use tokio::runtime::Handle;

use crate::Store;
use crate::error::Error;
use crate::live::DocumentBody;
use crate::live::Hub;
use crate::live::State;
use crate::sidebar_commands::{FileDone, FileError};

/// Reads the documents of chats' workspaces and of the host, for the editor panes.
#[derive(Clone)]
pub struct Documents {
    hub: Arc<Hub>,
    handle: Handle,
}

impl std::fmt::Debug for Documents {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter.debug_struct("Documents").finish_non_exhaustive()
    }
}

impl Documents {
    /// Reads `file`, a file of `chat`'s workspace or an absolute file of this computer, and
    /// hands its text to `done`; a binary file reads as Latin-1 text, as develop decodes it.
    ///
    /// # Errors
    /// [`Error::Stream`] while `chat` is not loaded yet; a read that fails reaches `done`.
    pub fn read(
        &self,
        chat: &str,
        file: &DocumentPath,
        done: FileDone<String>,
    ) -> Result<(), Error> {
        let (client, route) = self
            .hub
            .read(|state| owner_of(state, chat, file))
            .ok_or_else(|| Error::Stream(format!("chat {chat} is not loaded")))?;
        self.handle.spawn(async move {
            let body: Result<DocumentBody, FileError> = client
                .read(route)
                .await
                .map_err(|error| FileError::Daemon(error.into()));
            done(body.map(|body| text(&body)));
        });
        Ok(())
    }
}

impl Documents {
    /// Writes `text` as the whole content of `file`, as develop's `writeDocument` does, and
    /// tells `done` once the daemon holds it.
    ///
    /// # Errors
    /// [`Error::Stream`] while `chat` is not loaded; a write that fails reaches `done`.
    pub fn write(
        &self,
        (chat, file): (&str, &DocumentPath),
        text: &str,
        done: FileDone<()>,
    ) -> Result<(), Error> {
        let (client, body) = self
            .hub
            .read(|state| writer_of(state, (chat, file), text))
            .ok_or_else(|| Error::Stream(format!("chat {chat} is not loaded")))?;
        self.handle.spawn(async move {
            let written = client.send(daemonapi::Call::put(CONTENT).json(body)).await;
            done(
                written
                    .map(drop)
                    .map_err(|error| FileError::Daemon(error.into())),
            );
        });
        Ok(())
    }
}

impl Store {
    /// The editor panes' documents.
    pub fn documents(&self) -> Documents {
        Documents {
            hub: Arc::clone(&self.hub),
            handle: self.commands.handle.clone(),
        }
    }
}

fn text(body: &DocumentBody) -> String {
    if body.encoding != BASE64 {
        return body.content.clone();
    }
    base64::engine::general_purpose::STANDARD
        .decode(&body.content)
        .map_or_else(
            |_| body.content.clone(),
            |bytes| bytes.into_iter().map(char::from).collect(),
        )
}

fn owner_of(
    state: &State,
    chat: &str,
    file: &DocumentPath,
) -> Option<(daemonapi::Client, daemonapi::Route)> {
    if file.is_host() {
        let client = state.client_of_chat(chat).and(state.local_client())?;
        return Some((
            client,
            daemonapi::Route::host_document_content(file.as_str()),
        ));
    }
    let (client, workspace) = workspace_of(state, chat)?;
    let route = daemonapi::Route::document_content(&workspace, file.as_str());
    Some((client, route))
}

fn writer_of(
    state: &State,
    (chat, file): (&str, &DocumentPath),
    content: &str,
) -> Option<(daemonapi::Client, Value)> {
    let path = file.as_str();
    if file.is_host() {
        let client = state.client_of_chat(chat).and(state.local_client())?;
        return Some((client, json!({ "path": path, "content": content })));
    }
    let (client, workspace) = workspace_of(state, chat)?;
    let body = json!({ "workspaceId": workspace, "path": path, "content": content });
    Some((client, body))
}

fn workspace_of(
    state: &State,
    chat: &str,
) -> Option<(daemonapi::Client, String)> {
    let workspace = state
        .hosts
        .iter()
        .find_map(|slot| slot.store.chats.get(chat))
        .map(|record| record.workspace_id.clone())?;
    Some((state.client_of_chat(chat)?, workspace))
}

#[cfg(test)]
mod tests {
    use std::sync::mpsc;

    use super::*;
    use crate::test_support::{FakeDaemon, state};

    fn documents() -> (Documents, Arc<FakeDaemon>) {
        let (state, daemon) = state();
        let hub = Hub::new(state, Arc::new(|| {}));
        let runtime = Handle::current();
        (
            Documents {
                hub,
                handle: runtime,
            },
            daemon,
        )
    }

    fn read(
        documents: &Documents,
        (chat, path): (&str, &str),
    ) -> Result<Result<String, String>, Error> {
        let (sender, receiver) = mpsc::channel();
        let done: FileDone<String> = Box::new(move |result| {
            sender
                .send(result.map_err(|error| error.to_string()))
                .unwrap();
        });
        documents.read(chat, &DocumentPath::new(path).unwrap(), done)?;
        Ok(receiver.recv().unwrap())
    }

    fn both_read(documents: &Documents) -> Vec<Result<String, String>> {
        let files = [("hc", "src/a.ts"), ("hc", "logo.bin")];
        files
            .into_iter()
            .map(|file| read(documents, file).unwrap())
            .collect()
    }

    #[tokio::test(flavor = "multi_thread")]
    async fn a_workspace_file_is_read_from_the_chats_daemon_and_binary_reads_as_latin_1() {
        let (documents, daemon) = documents();
        daemon.ok(
            "GET",
            "/v0/documents/content?workspace=hw&path=src/a.ts",
            r#"{"content":"let a = 1"}"#,
        );
        daemon.ok(
            "GET",
            "/v0/documents/content?workspace=hw&path=logo.bin",
            r#"{"content":"AP9B","encoding":"base64"}"#,
        );
        let texts = tokio::task::spawn_blocking(move || both_read(&documents));
        let latin = "\u{0}\u{ff}A".to_owned();
        assert_eq!(texts.await.unwrap(), [Ok("let a = 1".to_owned()), Ok(latin)]);
    }

    fn write(
        documents: &Documents,
        chat: &str,
    ) -> Result<bool, Error> {
        let (sender, receiver) = mpsc::channel();
        let done: FileDone<()> = Box::new(move |result| sender.send(result.is_ok()).unwrap());
        let file = DocumentPath::new("src/a.ts").unwrap();
        documents.write((chat, &file), "b", done)?;
        Ok(receiver.recv().unwrap())
    }

    fn refused_then_written(documents: &Documents) -> (bool, bool) {
        let refused =
            write(documents, "nobody").is_err() && read(documents, ("nobody", "a.ts")).is_err();
        (refused, write(documents, "hc").unwrap())
    }

    #[tokio::test(flavor = "multi_thread")]
    async fn a_save_puts_the_whole_text_and_a_chat_not_loaded_is_refused_first() {
        let (documents, daemon) = documents();
        daemon.ok("PUT", "/v0/documents/content", r#"{"path":"src/a.ts"}"#);
        let written = tokio::task::spawn_blocking(move || refused_then_written(&documents));
        assert_eq!(
            written.await.unwrap(),
            (true, true),
            "refused, then written"
        );
        let seen = daemon.seen();
        let put = seen.iter().find(|call| call.method == "PUT").unwrap();
        let body = serde_json::json!({"workspaceId": "hw", "path": "src/a.ts", "content": "b"});
        assert_eq!(put.body, Some(body));
    }
}
