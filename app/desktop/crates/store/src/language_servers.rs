//! The language servers the daemon runs for chats' workspaces, reached as develop's
//! `lsp-client.ts` reaches them: REST routes of the chat that owns the workspace.

use std::sync::Arc;

use daemonapi::{Call, Route};
use serde_json::Value;
use tokio::runtime::Handle;

use crate::Store;
use crate::live::Hub;
use crate::live::State;
use crate::sidebar_commands::{FileDone, FileError};

/// Asks the language servers of the workspace a chat shows, for the editor panes.
#[derive(Clone)]
pub struct LanguageServers {
    hub: Arc<Hub>,
    handle: Handle,
}

impl std::fmt::Debug for LanguageServers {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("LanguageServers")
            .finish_non_exhaustive()
    }
}

impl LanguageServers {
    /// Posts `body` to the LSP route `verb` of the chat that owns `chat`'s workspace and hands
    /// the answer's data to `done`. A workspace with no language servers (the project home, or
    /// one not loaded yet) answers `null`, as develop's client does.
    pub fn post(
        &self,
        (chat, verb): (&str, &str),
        body: Value,
        done: FileDone<Value>,
    ) {
        let Some((client, owner)) = self.hub.read(|state| owner_of(state, chat)) else {
            done(Ok(Value::Null));
            return;
        };
        let call = Call::post(Route::lsp(&owner, verb)).json(body);
        self.handle.spawn(async move {
            let answer = client.send(call).await;
            done(answer.map_err(|error| FileError::Daemon(error.into())));
        });
    }

    /// Reads the state of the language server of the file at `path` in `chat`'s workspace and
    /// hands it to `done`; `null` where [`LanguageServers::post`] answers `null`.
    pub fn status(
        &self,
        (chat, path): (&str, &str),
        done: FileDone<Value>,
    ) {
        let Some((client, owner)) = self.hub.read(|state| owner_of(state, chat)) else {
            done(Ok(Value::Null));
            return;
        };
        let call = Call::get(Route::lsp_status(&owner, path));
        self.handle.spawn(async move {
            let answer = client.send(call).await;
            done(answer.map_err(|error| FileError::Daemon(error.into())));
        });
    }
}

impl Store {
    /// The editor panes' language servers.
    pub fn language_servers(&self) -> LanguageServers {
        LanguageServers {
            hub: Arc::clone(&self.hub),
            handle: self.commands.handle.clone(),
        }
    }
}

fn owner_of(
    state: &State,
    chat: &str,
) -> Option<(daemonapi::Client, String)> {
    let workspace = state.hosts.iter().find_map(|slot| {
        let record = slot.store.chats.get(chat)?;
        slot.store.workspaces.get(&record.workspace_id)
    })?;
    if workspace.repo_id.is_empty() || workspace.owning_chat_id.is_empty() {
        return None;
    }
    let owner = workspace.owning_chat_id.clone();
    Some((state.client_of_chat(chat)?, owner))
}

#[cfg(test)]
mod tests {
    use std::sync::mpsc;

    use serde_json::json;

    use super::*;
    use crate::test_support::{FakeDaemon, state};

    fn servers() -> (LanguageServers, Arc<FakeDaemon>) {
        let (state, daemon) = state();
        let hub = Hub::new(state, Arc::new(|| {}));
        let handle = Handle::current();
        (LanguageServers { hub, handle }, daemon)
    }

    async fn ask(
        servers: LanguageServers,
        ask: impl FnOnce(&LanguageServers, FileDone<Value>) + Send + 'static,
    ) -> Result<Value, String> {
        let asked = tokio::task::spawn_blocking(move || answer(|done| ask(&servers, done)));
        asked.await.unwrap()
    }

    fn answer(ask: impl FnOnce(FileDone<Value>)) -> Result<Value, String> {
        let (sender, receiver) = mpsc::channel();
        ask(Box::new(move |answer| {
            let answer = answer.map_err(|error| error.to_string());
            sender.send(answer).unwrap();
        }));
        receiver.recv().unwrap()
    }

    const HOVER: &str = "/v0/chats/bo/lsp/hover";

    const ANSWER: &str = r#"{"success":true,"data":{"contents":"x"}}"#;
    const REFUSAL: &str = r#"{"success":false}"#;

    type Case = (
        &'static str,
        (u16, &'static str),
        Result<Value, ()>,
        &'static [&'static str],
    );

    async fn hovered(
        chat: &'static str,
        (status, body): (u16, &'static str),
    ) -> (Result<Value, ()>, Vec<String>) {
        let (servers, daemon) = servers();
        daemon.answer(("POST", HOVER), status, body);
        let position = json!({"path": "a.ts", "position": {"line": 0, "character": 1}});
        let hover = ask(servers, move |s, d| s.post((chat, "hover"), position, d));
        let answer = hover.await.map_err(drop);
        let seen = daemon.seen().into_iter().map(|seen| seen.path).collect();
        (answer, seen)
    }

    #[test]
    fn a_pane_asks_the_servers_of_the_chat_that_owns_its_workspace() {
        let runtime = tokio::runtime::Runtime::new().unwrap();
        let cases: [Case; 3] = [
            ("t", (200, ANSWER), Ok(json!({"contents": "x"})), &[HOVER]),
            ("hc", (200, ANSWER), Ok(Value::Null), &[]),
            ("bo", (500, REFUSAL), Err(()), &[HOVER]),
        ];
        for (chat, daemon_answer, expected, asked) in cases {
            let (answer, seen) = runtime.block_on(hovered(chat, daemon_answer));
            assert_eq!(answer, expected, "{chat}");
            assert_eq!(seen, asked, "{chat}");
        }
    }

    #[tokio::test(flavor = "multi_thread")]
    async fn the_status_of_a_file_s_server_is_read_from_the_owning_chat() {
        let (servers, daemon) = servers();
        let path = "/v0/chats/bo/lsp/status?path=src/a%20b.ts";
        daemon.ok("GET", path, r#"{"state":"running"}"#);
        let status = ask(servers, |s, done| s.status(("t", "src/a b.ts"), done)).await;
        assert_eq!(status, Ok(json!({"state": "running"})));
    }
}
