//! The Files panel's changes to the workspace's files, each a daemon file route.

use std::path::Path;
use std::sync::Arc;

use serde_json::json;
use sidebar::{Command, Error};

use crate::call::Call;
use crate::copy_name;
use crate::daemon_client::DaemonClient;
use crate::daemon_read::read;
use crate::document_body::DocumentBody;
use crate::file_target::{self, FileTarget};
use crate::hub::Hub;
use crate::new_entry::NewEntry;
use crate::outcome::{Outcome, OutcomeSink};
use crate::panel_loader::load_directory;
use crate::plan::Plan;
use crate::query_encode::encode;
use crate::runner;
use crate::sidebar_commands::SidebarCommands;
use crate::state::State;

fn files_route(target: &FileTarget) -> String {
    format!("/v0/chats/{}/files", target.chat)
}

fn parent_of(path: &str) -> String {
    Path::new(path)
        .parent()
        .and_then(Path::to_str)
        .unwrap_or_default()
        .to_owned()
}

async fn change_and_reload(
    hub: &Hub,
    plan: Plan,
    outcome: &OutcomeSink,
    (client, chat, directory): (Option<DaemonClient>, String, String),
) {
    runner::run(hub, plan, outcome).await;
    if let Some(client) = client {
        load_directory(hub, &client, (&chat, &directory)).await;
    }
}

async fn say_content(
    client: &DaemonClient,
    route: String,
    outcome: &OutcomeSink,
) {
    let said = match read::<DocumentBody>(client, route).await {
        Ok(body) => Outcome::CopyText(body.content),
        Err(error) => Outcome::Failed(error.to_string()),
    };
    outcome(said);
}

fn refused(reason: String) -> Error {
    Error::Refused { reason }
}

fn name_of(path: &str) -> &str {
    Path::new(path)
        .file_name()
        .and_then(std::ffi::OsStr::to_str)
        .unwrap_or(path)
}

fn taken_in(
    state: &State,
    directory: &str,
) -> Vec<String> {
    state
        .panels
        .listings
        .get(directory)
        .map_or_default(|entries| entries.iter().map(|entry| entry.path.clone()).collect())
}

impl SidebarCommands {
    fn require_free(
        &self,
        path: &str,
    ) -> Result<(), Error> {
        let taken = self.hub.read(|state| taken_in(state, &parent_of(path)));
        if taken.iter().any(|held| held == path) {
            return Err(refused(format!("{} already exists", name_of(path))));
        }
        Ok(())
    }

    fn free_copy_of(
        &self,
        path: &str,
    ) -> String {
        let directory = parent_of(path);
        let taken = self.hub.read(|state| taken_in(state, &directory));
        let names: Vec<&str> = taken.iter().map(|held| name_of(held)).collect();
        let copy = copy_name::free(name_of(path), &names);
        Path::new(&directory)
            .join(copy)
            .to_string_lossy()
            .into_owned()
    }

    fn refresh_directory(
        &self,
        path: &str,
    ) -> Result<(), Error> {
        let target = self.hub.read(file_target::of).map_err(Error::from)?;
        let client = self
            .hub
            .read(|state| state.client_of_chat(&target.chat))
            .ok_or_else(|| refused("this workspace is not loaded yet".to_owned()))?;
        let (hub, path) = (Arc::clone(&self.hub), path.to_owned());
        self.handle
            .spawn(async move { load_directory(&hub, &client, (&target.chat, &path)).await });
        Ok(())
    }

    pub(crate) fn file_command(
        &self,
        command: &Command,
    ) -> Option<Result<(), Error>> {
        Some(match command {
            Command::CreateFile { path } => self.create_new(path, NewEntry::File),
            Command::CreateDirectory { path } => self.create_new(path, NewEntry::Directory),
            Command::DuplicateFile { path } => self.duplicate_file(path, &self.free_copy_of(path)),
            Command::RenameFile { path, new_path } => self
                .require_free(new_path)
                .and_then(|()| self.rename_file(path, new_path)),
            Command::DeleteFile { path } => self.delete_file(path),
            Command::CopyFile { path, destination } => self
                .require_free(destination)
                .and_then(|()| self.duplicate_file(path, destination)),
            Command::CopyFileContent { path } => self.copy_file_content(path),
            Command::RefreshDirectory { path } => self.refresh_directory(path),
            Command::CollapseDirectory { path } => {
                self.collapse_directory(path);
                Ok(())
            },
            _ => return None,
        })
    }

    fn create_new(
        &self,
        path: &str,
        entry: NewEntry,
    ) -> Result<(), Error> {
        self.require_free(path)?;
        self.create_entry(path, entry)
    }

    fn change_files(
        &self,
        make: impl FnOnce(&FileTarget) -> (Call, String),
    ) -> Result<(), Error> {
        let target = self.hub.read(file_target::of).map_err(Error::from)?;
        let (call, reload) = make(&target);
        let plan = Plan::of(&target.host, vec![call]);
        let (hub, outcome) = (Arc::clone(&self.hub), Arc::clone(&self.outcome));
        let client = self.hub.read(|state| state.client_of_chat(&target.chat));
        let reload = (client, target.chat, reload);
        self.handle
            .spawn(async move { change_and_reload(&hub, plan, &outcome, reload).await });
        Ok(())
    }

    /// Makes an empty file or directory at the workspace-relative `path`.
    ///
    /// # Errors
    /// [`Error::Refused`] when no workspace is open.
    pub fn create_entry(
        &self,
        path: &str,
        entry: NewEntry,
    ) -> Result<(), Error> {
        self.change_files(|target| {
            let body = json!({"path": path, "type": entry.kind()});
            (Call::post(files_route(target)).json(body), parent_of(path))
        })
    }

    /// Copies `source` to `destination`, both workspace-relative.
    ///
    /// # Errors
    /// [`Error::Refused`] when no workspace is open.
    pub fn duplicate_file(
        &self,
        source: &str,
        destination: &str,
    ) -> Result<(), Error> {
        self.change_files(|target| {
            let body = json!({"sourcePath": source, "destPath": destination});
            let route = format!("{}/copy", files_route(target));
            (Call::post(route).json(body), parent_of(destination))
        })
    }

    /// Renames `path` to `new_path`, both workspace-relative.
    ///
    /// # Errors
    /// [`Error::Refused`] when no workspace is open.
    pub fn rename_file(
        &self,
        path: &str,
        new_path: &str,
    ) -> Result<(), Error> {
        self.change_files(|target| {
            let body = json!({"path": path, "newPath": new_path});
            (
                Call::patch(files_route(target)).json(body),
                parent_of(new_path),
            )
        })
    }

    /// Deletes the workspace-relative `path`.
    ///
    /// # Errors
    /// [`Error::Refused`] when no workspace is open.
    pub fn delete_file(
        &self,
        path: &str,
    ) -> Result<(), Error> {
        self.change_files(|target| {
            let body = json!({"path": path});
            (
                Call::delete(files_route(target)).json(body),
                parent_of(path),
            )
        })
    }

    /// Reads the workspace-relative `path`; the shell is handed its text to put on the clipboard.
    ///
    /// # Errors
    /// [`Error::Refused`] when no workspace is open.
    pub fn copy_file_content(
        &self,
        path: &str,
    ) -> Result<(), Error> {
        let target = self.hub.read(file_target::of).map_err(Error::from)?;
        let client = self
            .hub
            .read(|state| state.client_of_chat(&target.chat))
            .ok_or_else(|| refused("this workspace is not loaded yet".to_owned()))?;
        let route = format!(
            "/v0/documents/content?workspace={}&path={}",
            encode(&target.workspace),
            encode(path)
        );
        let outcome = Arc::clone(&self.outcome);
        self.handle
            .spawn(async move { say_content(&client, route, &outcome).await });
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Mutex;

    use tokio::runtime::Handle;

    use super::*;
    use crate::fleet::Fleet;
    use crate::hub::Hub;
    use crate::test_support::{FakeDaemon, state};

    type Said = Arc<Mutex<Vec<Outcome>>>;

    fn commands() -> (SidebarCommands, Arc<FakeDaemon>, Said) {
        let (mut state, daemon) = state();
        state.panels.follow(Some("hc".into()));
        let transport: Arc<dyn protocol::DaemonTransport> = Arc::<FakeDaemon>::clone(&daemon);
        let hub = Hub::new(state, Arc::new(|| {}));
        let said = Said::default();
        let sink = Arc::clone(&said);
        let commands = SidebarCommands {
            fleet: Fleet::new(&Handle::current(), &hub, &transport),
            hub,
            handle: Handle::current(),
            outcome: Arc::new(move |outcome| sink.lock().unwrap().push(outcome)),
        };
        (commands, daemon, said)
    }

    fn answer_files(
        daemon: &FakeDaemon,
        files: &str,
    ) {
        daemon.ok("POST", files, "{}");
        daemon.ok("POST", &format!("{files}/copy"), "{}");
        daemon.ok("PATCH", files, "{}");
        daemon.ok("DELETE", files, "{}");
        daemon.ok("GET", &format!("{files}/tree?path=src"), "[]");
    }

    async fn until_seen(
        daemon: &FakeDaemon,
        method: &str,
        path: &str,
    ) -> Option<serde_json::Value> {
        let mut body = None;
        while body.is_none() {
            let call = daemon
                .seen()
                .into_iter()
                .find(|call| call.method == method && call.path == path);
            body = call.and_then(|call| call.body);
            tokio::task::yield_now().await;
        }
        body
    }

    async fn until_said_count(
        said: &Said,
        count: usize,
    ) {
        while said.lock().unwrap().len() < count {
            tokio::task::yield_now().await;
        }
    }

    async fn until_said(said: &Said) -> Outcome {
        while said.lock().unwrap().is_empty() {
            tokio::task::yield_now().await;
        }
        said.lock().unwrap()[0].clone()
    }

    async fn each_file_change_calls_its_route_with_workspace_relative_paths() {
        let (commands, daemon, _) = commands();
        let files = "/v0/chats/hc/files";
        answer_files(&daemon, files);
        commands
            .create_entry("src/new", NewEntry::Directory)
            .unwrap();
        commands.duplicate_file("src/a", "src/b").unwrap();
        commands.rename_file("src/b", "src/c").unwrap();
        commands.delete_file("src/c").unwrap();
        let made = until_seen(&daemon, "POST", files).await;
        let copied = until_seen(&daemon, "POST", &format!("{files}/copy")).await;
        let moved = until_seen(&daemon, "PATCH", files).await;
        let gone = until_seen(&daemon, "DELETE", files).await;
        assert_eq!(made, Some(json!({"path": "src/new", "type": "dir"})));
        assert_eq!(
            copied,
            Some(json!({"sourcePath": "src/a", "destPath": "src/b"}))
        );
        assert_eq!(moved, Some(json!({"path": "src/b", "newPath": "src/c"})));
        assert_eq!(gone, Some(json!({"path": "src/c"})));
    }

    async fn a_files_content_reaches_the_shell_and_a_refusal_is_said() {
        let (commands, daemon, said) = commands();
        daemon.ok(
            "GET",
            "/v0/documents/content?workspace=hw&path=a%20b.txt",
            r#"{"content":"hello","encoding":"utf-8"}"#,
        );
        commands.copy_file_content("a b.txt").unwrap();
        assert_eq!(until_said(&said).await, Outcome::CopyText("hello".into()));
        commands.copy_file_content("missing").unwrap();
        until_said_count(&said, 2).await;
        assert!(matches!(said.lock().unwrap()[1], Outcome::Failed(_)));
    }

    fn listed(commands: &SidebarCommands) {
        let entries = serde_json::from_value(json!([
            {"name": "a.rs", "path": "src/a.rs", "type": "file"},
            {"name": "a copy.rs", "path": "src/a copy.rs", "type": "file"}
        ]))
        .unwrap();
        commands
            .hub
            .edit(|state| state.panels.list("hc", "src", entries));
    }

    async fn the_menu_commands_refuse_a_name_in_use_and_pick_a_free_copy_name() {
        let (commands, daemon, _) = commands();
        listed(&commands);
        answer_files(&daemon, "/v0/chats/hc/files");
        let taken = Command::CreateFile {
            path: "src/a.rs".into(),
        };
        let refusal = commands.file_command(&taken).unwrap().unwrap_err();
        assert!(refusal.to_string().contains("a.rs already exists"));
        let rename = Command::RenameFile {
            path: "src/b.rs".into(),
            new_path: "src/a.rs".into(),
        };
        assert!(commands.file_command(&rename).unwrap().is_err());
        let paste = Command::CopyFile {
            path: "src/a.rs".into(),
            destination: "src/a.rs".into(),
        };
        assert!(commands.file_command(&paste).unwrap().is_err());
        let copy = Command::DuplicateFile {
            path: "src/a.rs".into(),
        };
        commands.file_command(&copy).unwrap().unwrap();
        let copied = until_seen(&daemon, "POST", "/v0/chats/hc/files/copy").await;
        assert_eq!(
            copied,
            Some(json!({"sourcePath": "src/a.rs", "destPath": "src/a copy 2.rs"}))
        );
        assert!(commands.file_command(&Command::OpenSettings).is_none());
    }

    fn without_an_open_workspace_a_file_change_is_refused() {
        let (commands, _, _) = commands();
        commands.hub.edit(|state| state.panels.follow(None));
        assert!(commands.delete_file("a").is_err());
        assert!(commands.copy_file_content("a").is_err());
    }

    type Case = fn() -> std::pin::Pin<Box<dyn Future<Output = ()>>>;

    const CASES: [Case; 4] = [
        || {
            without_an_open_workspace_a_file_change_is_refused();
            Box::pin(async {})
        },
        || Box::pin(each_file_change_calls_its_route_with_workspace_relative_paths()),
        || Box::pin(a_files_content_reaches_the_shell_and_a_refusal_is_said()),
        || Box::pin(the_menu_commands_refuse_a_name_in_use_and_pick_a_free_copy_name()),
    ];

    async fn run_all() {
        for case in CASES {
            case().await;
        }
    }

    #[tokio::test]
    async fn every_case_ends_as_it_says() {
        run_all().await;
    }
}
