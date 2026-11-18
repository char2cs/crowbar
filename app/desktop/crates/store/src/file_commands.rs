//! The Files panel's menu commands, each checked against the listing and run as a file change.

use std::path::Path;
use std::sync::Arc;

use sidebar::{Command, Error};

use crate::copy_name;
use crate::file_session::parent_of;
use crate::file_target;
use crate::new_entry::NewEntry;
use crate::panel_loader::load_directory;
use crate::plan_error::PlanError;
use crate::sidebar_commands::SidebarCommands;
use crate::state::State;

fn already_exists(path: &str) -> Error {
    Error::Refused {
        reason: format!("{} already exists", name_of(path)),
    }
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
            return Err(already_exists(path));
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
            .ok_or(PlanError::WorkspaceNotLoaded)?;
        let (hub, path) = (Arc::clone(&self.hub), path.to_owned());
        self.handle
            .spawn(async move { load_directory(&hub, &client, (&target.chat, &path)).await });
        Ok(())
    }

    fn create_new(
        &self,
        path: &str,
        entry: NewEntry,
    ) -> Result<(), Error> {
        self.require_free(path)?;
        self.create_entry(path, entry)
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
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::test_support::{FakeDaemon, followed_commands};

    fn commands() -> (SidebarCommands, Arc<FakeDaemon>) {
        followed_commands(Arc::new(|_| {}), &Arc::default())
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

    async fn until_copied(daemon: &FakeDaemon) -> Option<serde_json::Value> {
        let mut body = None;
        while body.is_none() {
            let call = daemon
                .seen()
                .into_iter()
                .find(|call| call.method == "POST" && call.path.ends_with("/files/copy"));
            body = call.and_then(|call| call.body);
            tokio::task::yield_now().await;
        }
        body
    }

    #[tokio::test]
    async fn the_menu_commands_refuse_a_name_in_use_and_pick_a_free_copy_name() {
        let (commands, daemon) = commands();
        listed(&commands);
        daemon.ok("POST", "/v0/chats/hc/files/copy", "{}");
        daemon.ok("GET", "/v0/chats/hc/files/tree?path=src", "[]");
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
        assert_eq!(
            until_copied(&daemon).await,
            Some(json!({"sourcePath": "src/a.rs", "destPath": "src/a copy 2.rs"}))
        );
        assert!(commands.file_command(&Command::OpenSettings).is_none());
    }

    #[tokio::test]
    async fn without_an_open_workspace_a_file_command_is_refused() {
        let (commands, _) = commands();
        commands.hub.edit(|state| state.panels.follow(None));
        let delete = Command::DeleteFile { path: "a".into() };
        let refresh = Command::RefreshDirectory {
            path: String::new(),
        };
        assert!(commands.file_command(&delete).unwrap().is_err());
        assert!(commands.file_command(&refresh).unwrap().is_err());
    }
}
