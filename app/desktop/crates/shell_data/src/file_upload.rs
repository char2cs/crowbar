//! Copying local files into a directory of the followed workspace.

use std::path::{Path, PathBuf};

use sidebar::Error;

use crate::file_done::FileDone;
use crate::file_session::{FileSession, join};
use crate::hub::Hub;
use crate::sidebar_commands::SidebarCommands;

fn name_of(source: &Path) -> Result<String, String> {
    source
        .file_name()
        .map(|name| name.to_string_lossy().into_owned())
        .ok_or_else(|| format!("{} has no file name", source.display()))
}

async fn copy_one(
    session: &FileSession,
    (directory, source): (&str, &Path),
) -> Result<String, String> {
    let destination = join(directory, &name_of(source)?);
    let owned = source.to_path_buf();
    let bytes = tokio::task::spawn_blocking(move || std::fs::read(owned))
        .await
        .map_err(|error| error.to_string())?
        .map_err(|error| format!("{}: {error}", source.display()))?;
    session
        .create_file(&destination)
        .await
        .map_err(|error| error.to_string())?;
    session
        .write_bytes(&destination, &bytes)
        .await
        .map_err(|error| error.to_string())?;
    Ok(destination)
}

async fn copy_all(
    (hub, session): (&Hub, &FileSession),
    directory: &str,
    sources: &[PathBuf],
) -> Result<Vec<String>, String> {
    let mut written = Vec::new();
    for source in sources {
        let copied = copy_one(session, (directory, source)).await;
        session.reload(hub, directory).await;
        written.push(copied?);
    }
    Ok(written)
}

impl SidebarCommands {
    /// Copies each local file of `sources` into `directory` (the workspace root when empty) one
    /// after the other, then reloads the listing. `done` gets the workspace-relative paths
    /// written, or the first failure; the files before it stay.
    ///
    /// # Errors
    /// [`Error::Refused`] when no workspace is open.
    pub fn upload_files(
        &self,
        directory: &str,
        sources: Vec<PathBuf>,
        done: FileDone<Vec<String>>,
    ) -> Result<(), Error> {
        let session = FileSession::of(&self.hub).map_err(Error::from)?;
        let (hub, directory) = (std::sync::Arc::clone(&self.hub), directory.to_owned());
        self.handle.spawn(async move {
            done(copy_all((&hub, &session), &directory, &sources).await);
        });
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

    async fn upload(
        commands: &SidebarCommands,
        sources: Vec<PathBuf>,
    ) -> Result<Vec<String>, String> {
        let (sender, receiver) = mpsc::channel();
        let done: FileDone<Vec<String>> = Box::new(move |result| {
            sender.send(result).unwrap();
        });
        commands.upload_files("assets", sources, done).unwrap();
        tokio::task::spawn_blocking(move || receiver.recv().unwrap())
            .await
            .unwrap()
    }

    fn answer_writes(daemon: &FakeDaemon) {
        daemon.ok("POST", "/v0/chats/hc/files", "{}");
        daemon.ok("PUT", "/v0/documents/content", "{}");
        daemon.ok("GET", "/v0/chats/hc/files/tree?path=assets", "[]");
    }

    #[tokio::test]
    async fn a_local_file_is_created_then_written_as_base64_and_the_listing_reloads() {
        let (commands, daemon) = commands();
        answer_writes(&daemon);
        let dir = tempfile::tempdir().unwrap();
        let source = dir.path().join("logo.bin");
        std::fs::write(&source, [0_u8, 255, 10]).unwrap();
        let written = upload(&commands, vec![source]).await.unwrap();
        assert_eq!(written, ["assets/logo.bin"]);
        let seen = daemon.seen();
        let bodies: Vec<_> = seen.iter().filter_map(|call| call.body.clone()).collect();
        assert_eq!(
            bodies,
            [
                json!({"path": "assets/logo.bin", "type": "file"}),
                json!({
                    "workspaceId": "hw",
                    "path": "assets/logo.bin",
                    "content": "AP8K",
                    "encoding": "base64"
                }),
            ]
        );
        assert_eq!(
            seen.last().map(|call| call.path.as_str()),
            Some("/v0/chats/hc/files/tree?path=assets"),
            "the listing reloads"
        );
    }

    #[tokio::test]
    async fn a_source_that_cannot_be_copied_stops_the_upload_and_says_why() {
        let (commands, daemon) = commands();
        answer_writes(&daemon);
        let gone = upload(&commands, vec![PathBuf::from("/definitely/not/here.txt")]).await;
        let nameless = upload(&commands, vec![PathBuf::from("/")]).await;
        assert!(gone.unwrap_err().contains("/definitely/not/here.txt"));
        assert!(nameless.unwrap_err().contains("has no file name"));
        assert!(daemon.seen().iter().all(|call| call.method != "POST"));
    }
}
