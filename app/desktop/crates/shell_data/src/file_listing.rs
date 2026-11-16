//! Every file under a directory of the followed workspace.

use sidebar::Error;

use crate::file_done::FileDone;
use crate::file_entry::FileEntry;
use crate::file_session::FileSession;
use crate::sidebar_commands::SidebarCommands;

const ALWAYS_HIDDEN: &str = ".ds_store";

fn paths_by_kind(entries: Vec<FileEntry>) -> (Vec<String>, Vec<String>) {
    let (directories, files): (Vec<_>, Vec<_>) = entries
        .into_iter()
        .filter(|entry| entry.name.to_lowercase() != ALWAYS_HIDDEN)
        .partition(FileEntry::is_directory);
    let paths = |entries: Vec<FileEntry>| entries.into_iter().map(|entry| entry.path).collect();
    (paths(directories), paths(files))
}

async fn walk(
    session: &FileSession,
    root: &str,
) -> Result<Vec<String>, String> {
    let mut directories = vec![root.to_owned()];
    let mut files = Vec::new();
    while let Some(directory) = directories.pop() {
        let entries = session
            .list(&directory)
            .await
            .map_err(|error| error.to_string())?;
        let (more_directories, more_files) = paths_by_kind(entries);
        directories.extend(more_directories);
        files.extend(more_files);
    }
    files.sort();
    Ok(files)
}

impl SidebarCommands {
    /// Lists the workspace-relative paths of every file under `directory` (the workspace root
    /// when empty), sorted, through the daemon's tree route.
    ///
    /// # Errors
    /// [`Error::Refused`] when no workspace is open; a listing that fails reaches `done`.
    pub fn list_files(
        &self,
        directory: &str,
        done: FileDone<Vec<String>>,
    ) -> Result<(), Error> {
        let session = FileSession::of(&self.hub).map_err(Error::from)?;
        let directory = directory.to_owned();
        self.handle
            .spawn(async move { done(walk(&session, &directory).await) });
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use std::sync::{Arc, mpsc};

    use serde_json::json;

    use tokio::runtime::Handle;

    use super::*;
    use crate::fleet::Fleet;
    use crate::hub::Hub;
    use crate::test_support::{FakeDaemon, state};

    fn commands() -> (SidebarCommands, Arc<FakeDaemon>) {
        let (mut state, daemon) = state();
        state.panels.follow(Some("hc".into()));
        let transport: Arc<dyn protocol::DaemonTransport> = Arc::<FakeDaemon>::clone(&daemon);
        let hub = Hub::new(state, Arc::new(|| {}));
        let commands = SidebarCommands {
            fleet: Fleet::new(&Handle::current(), &hub, &transport),
            hub,
            handle: Handle::current(),
            outcome: Arc::new(|_| {}),
        };
        (commands, daemon)
    }

    #[tokio::test]
    async fn every_file_under_a_directory_is_listed_sorted_and_the_system_noise_is_skipped() {
        let (commands, daemon) = commands();
        let top = json!([
            {"name": "z.rs", "path": "src/z.rs", "type": "file"},
            {"name": "inner", "path": "src/inner", "type": "directory"},
            {"name": ".DS_Store", "path": "src/.DS_Store", "type": "file"}
        ]);
        let inner = json!([{"name": "a.rs", "path": "src/inner/a.rs", "type": "file"}]);
        daemon.ok("GET", "/v0/chats/hc/files/tree?path=src", &top.to_string());
        daemon.ok(
            "GET",
            "/v0/chats/hc/files/tree?path=src/inner",
            &inner.to_string(),
        );
        let (sender, receiver) = mpsc::channel();
        let done: FileDone<Vec<String>> = Box::new(move |result| sender.send(result).unwrap());
        commands.list_files("src", done).unwrap();
        let listed = tokio::task::spawn_blocking(move || receiver.recv().unwrap())
            .await
            .unwrap();
        assert_eq!(listed.unwrap(), ["src/inner/a.rs", "src/z.rs"]);
    }

    #[tokio::test]
    async fn without_an_open_workspace_nothing_is_listed() {
        let (commands, _) = commands();
        commands.hub.edit(|state| state.panels.follow(None));
        assert!(commands.list_files("", Box::new(drop)).is_err());
    }
}
