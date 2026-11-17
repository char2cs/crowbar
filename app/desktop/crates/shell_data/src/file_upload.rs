//! Copying local files into a directory of the followed workspace.

use std::path::{Path, PathBuf};
use std::sync::Arc;

use sidebar::Error;

use crate::file_done::FileDone;
use crate::file_error::FileError;
use crate::file_session::{FileSession, join};
use crate::file_store::FileStore;
use crate::sidebar_commands::SidebarCommands;

fn name_of(source: &Path) -> Result<String, FileError> {
    source
        .file_name()
        .map(|name| name.to_string_lossy().into_owned())
        .ok_or_else(|| FileError::NoFileName(source.to_path_buf()))
}

async fn read_local(
    files: &Arc<dyn FileStore>,
    source: &Path,
) -> Result<Vec<u8>, FileError> {
    let (files, owned) = (Arc::clone(files), source.to_path_buf());
    let read = tokio::task::spawn_blocking(move || files.read(&owned)).await?;
    read.map_err(|cause| FileError::Local {
        path: source.to_path_buf(),
        source: cause,
    })
}

async fn copy_one(
    (session, files): (&FileSession, &Arc<dyn FileStore>),
    (directory, source): (&str, &Path),
) -> Result<String, FileError> {
    let destination = join(directory, &name_of(source)?);
    let bytes = read_local(files, source).await?;
    session.create_file(&destination).await?;
    session.write_bytes(&destination, &bytes).await?;
    Ok(destination)
}

async fn copy_all(
    (session, files): (&FileSession, &Arc<dyn FileStore>),
    (directory, sources): (&str, &[PathBuf]),
) -> Result<Vec<String>, FileError> {
    let mut copies = Vec::new();
    let copied = copy_into(&mut copies, (session, files), (directory, sources)).await;
    copied.map(|()| copies)
}

async fn copy_into(
    copies: &mut Vec<String>,
    (session, files): (&FileSession, &Arc<dyn FileStore>),
    (directory, sources): (&str, &[PathBuf]),
) -> Result<(), FileError> {
    for source in sources {
        copies.push(copy_one((session, files), (directory, source)).await?);
    }
    Ok(())
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
        let (hub, files) = (Arc::clone(&self.hub), Arc::clone(&self.files));
        let directory = directory.to_owned();
        self.handle.spawn(async move {
            let copied = copy_all((&session, &files), (&directory, &sources)).await;
            session.reload(&hub, &directory).await;
            done(copied);
        });
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use std::sync::mpsc;

    use serde_json::json;

    use super::*;
    use crate::test_support::{FakeDaemon, FakeFiles, followed_commands};

    fn commands() -> (SidebarCommands, Arc<FakeDaemon>, Arc<FakeFiles>) {
        let files = Arc::new(FakeFiles::default());
        let (commands, daemon) = followed_commands(Arc::new(|_| {}), &files);
        (commands, daemon, files)
    }

    async fn upload(
        commands: &SidebarCommands,
        sources: Vec<PathBuf>,
    ) -> Result<Vec<String>, FileError> {
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
        let (commands, daemon, files) = commands();
        answer_writes(&daemon);
        files.put("/local/logo.bin", &[0_u8, 255, 10]);
        let written = upload(&commands, vec![PathBuf::from("/local/logo.bin")])
            .await
            .unwrap();
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
        let (commands, daemon, _) = commands();
        answer_writes(&daemon);
        let gone = upload(&commands, vec![PathBuf::from("/definitely/not/here.txt")]).await;
        let nameless = upload(&commands, vec![PathBuf::from("/")]).await;
        assert!(
            gone.unwrap_err()
                .to_string()
                .contains("/definitely/not/here.txt")
        );
        assert!(matches!(nameless, Err(FileError::NoFileName(_))));
        assert!(daemon.seen().iter().all(|call| call.method != "POST"));
    }
}
