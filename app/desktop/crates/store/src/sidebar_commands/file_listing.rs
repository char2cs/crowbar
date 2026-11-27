use sidebar::Error;

use crate::live::FileEntry;
use crate::sidebar_commands::SidebarCommands;
use crate::sidebar_commands::file_done::FileDone;
use crate::sidebar_commands::file_error::FileError;
use crate::sidebar_commands::file_session::FileSession;

const ALWAYS_HIDDEN: &str = ".ds_store";

fn paths_by_kind(entries: Vec<FileEntry>) -> (Vec<String>, Vec<String>) {
    let (directories, files): (Vec<_>, Vec<_>) = entries
        .into_iter()
        .filter(|entry| entry.name.to_lowercase() != ALWAYS_HIDDEN)
        .partition(FileEntry::is_directory);
    let paths = |entries: Vec<FileEntry>| entries.into_iter().map(|entry| entry.path).collect();
    (paths(directories), paths(files))
}

fn sorted(mut paths: Vec<String>) -> Vec<String> {
    paths.sort();
    paths
}

async fn walk(
    session: &FileSession,
    root: &str,
) -> Result<Vec<String>, FileError> {
    let mut directories = vec![root.to_owned()];
    let mut files = Vec::new();
    while let Some(directory) = directories.pop() {
        let (more_directories, more_files) = paths_by_kind(session.list(&directory).await?);
        directories.extend(more_directories);
        files.extend(more_files);
    }
    Ok(sorted(files))
}

impl SidebarCommands {
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

    use super::*;
    use crate::test_support::{FakeDaemon, followed_commands};

    fn commands() -> (SidebarCommands, Arc<FakeDaemon>) {
        followed_commands(Arc::new(|_| {}), &Arc::default())
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
