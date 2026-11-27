use std::path::{Path, PathBuf};
use std::sync::Arc;

use futures_util::{StreamExt as _, TryStreamExt as _};
use sidebar::Error;

use crate::live::Files;
use crate::live::Hub;
use crate::sidebar_commands::SidebarCommands;
use crate::sidebar_commands::file_done::FileDone;
use crate::sidebar_commands::file_error::FileError;
use crate::sidebar_commands::file_session::{FileSession, join};

fn name_of(source: &Path) -> Result<String, FileError> {
    source
        .file_name()
        .map(|name| name.to_string_lossy().into_owned())
        .ok_or_else(|| FileError::NoFileName(source.to_path_buf()))
}

async fn read_local(
    files: &Arc<dyn Files>,
    source: &Path,
) -> Result<Vec<u8>, FileError> {
    let (files, owned) = (Arc::clone(files), source.to_path_buf());
    let read = tokio::task::spawn_blocking(move || files.read(&owned)).await?;
    read.map_err(|cause| FileError::Local {
        path: source.to_path_buf(),
        source: cause,
    })
}

struct FileUpload {
    session: FileSession,
    hub: Arc<Hub>,
    files: Arc<dyn Files>,
    directory: String,
}

impl FileUpload {
    async fn copy_one(
        &self,
        source: &Path,
    ) -> Result<String, FileError> {
        let destination = join(&self.directory, &name_of(source)?);
        let bytes = read_local(&self.files, source).await?;
        self.session.create_file(&destination).await?;
        self.session.write_bytes(&destination, &bytes).await?;
        Ok(destination)
    }

    async fn run(
        self,
        sources: Vec<PathBuf>,
        done: FileDone<Vec<String>>,
    ) {
        let copied = futures_util::stream::iter(&sources)
            .then(|source| self.copy_one(source))
            .try_collect()
            .await;
        self.session.reload(&self.hub, &self.directory).await;
        done(copied);
    }
}

impl SidebarCommands {
    pub fn upload_files(
        &self,
        directory: &str,
        sources: Vec<PathBuf>,
        done: FileDone<Vec<String>>,
    ) -> Result<(), Error> {
        let upload = FileUpload {
            session: FileSession::of(&self.hub).map_err(Error::from)?,
            hub: Arc::clone(&self.hub),
            files: Arc::clone(&self.files),
            directory: directory.to_owned(),
        };
        self.handle.spawn(upload.run(sources, done));
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
