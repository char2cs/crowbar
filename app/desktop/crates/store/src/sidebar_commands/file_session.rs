use std::path::Path;

use daemonapi::Call;
use serde_json::json;

use crate::error::Result;
use crate::live::DocumentBody;
use crate::live::FileEntry;
use crate::live::Hub;
use crate::live::PlanError;
use crate::live::{FileTarget, panel_loader::file_target};
use crate::live::{load_directory, tree_path};

const NOT_FOUND: u16 = 404;
const BASE64: &str = "base64";
pub(crate) const PATH_KEY: &str = "path";

pub(crate) fn parent_of(path: &str) -> String {
    Path::new(path)
        .parent()
        .and_then(Path::to_str)
        .unwrap_or_default()
        .to_owned()
}

pub(crate) fn join(
    directory: &str,
    name: &str,
) -> String {
    Path::new(directory)
        .join(name)
        .to_string_lossy()
        .into_owned()
}

#[derive(Debug, Clone)]
pub(crate) struct FileSession {
    client: daemonapi::Client,
    target: FileTarget,
}

impl FileSession {
    pub(crate) fn of(hub: &Hub) -> std::result::Result<Self, PlanError> {
        let target = hub.read(file_target::of)?;
        let client = hub
            .read(|state| state.client_of_chat(&target.chat))
            .ok_or(PlanError::WorkspaceNotLoaded)?;
        Ok(Self { client, target })
    }

    pub(crate) async fn list(
        &self,
        directory: &str,
    ) -> Result<Vec<FileEntry>> {
        Ok(self
            .client
            .read(tree_path(&self.target.chat, directory))
            .await?)
    }

    pub(crate) async fn reload(
        &self,
        hub: &Hub,
        directory: &str,
    ) {
        load_directory(hub, &self.client, (&self.target.chat, directory)).await;
    }

    pub(crate) async fn create_file(
        &self,
        path: &str,
    ) -> Result<()> {
        let route = format!("/v0/chats/{}/files", self.target.chat);
        let body = json!({PATH_KEY: path, "type": "file"});
        self.client.send(Call::post(route).json(body)).await?;
        Ok(())
    }

    pub(crate) async fn read_text(
        &self,
        path: &str,
    ) -> Result<String> {
        let body: DocumentBody = self.client.read(self.content_route(path)).await?;
        Ok(body.content)
    }

    pub(crate) async fn exists(
        &self,
        path: &str,
    ) -> Result<bool> {
        match self.client.send(Call::get(self.content_route(path))).await {
            Ok(_) => Ok(true),
            Err(daemonapi::Error::Refused { status, .. }) if status == NOT_FOUND => Ok(false),
            Err(error) => Err(error.into()),
        }
    }

    pub(crate) async fn write_text(
        &self,
        path: &str,
        text: &str,
    ) -> Result<()> {
        let body = json!({"workspaceId": self.target.workspace, PATH_KEY: path, "content": text});
        self.write(body).await
    }

    pub(crate) async fn write_bytes(
        &self,
        path: &str,
        bytes: &[u8],
    ) -> Result<()> {
        use base64::Engine as _;
        let content = base64::engine::general_purpose::STANDARD.encode(bytes);
        let body = json!({
            "workspaceId": self.target.workspace,
            PATH_KEY: path,
            "content": content,
            "encoding": BASE64
        });
        self.write(body).await
    }

    async fn write(
        &self,
        body: serde_json::Value,
    ) -> Result<()> {
        self.client
            .send(Call::put("/v0/documents/content").json(body))
            .await?;
        Ok(())
    }

    fn content_route(
        &self,
        path: &str,
    ) -> daemonapi::Route {
        daemonapi::Route::document_content(&self.target.workspace, path)
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;
    use crate::test_support::{FakeDaemon, state};

    fn session() -> (FileSession, Arc<FakeDaemon>) {
        let (mut state, daemon) = state();
        state.panels.follow(Some("hc".into()));
        let hub = Hub::new(state, Arc::new(|| {}));
        (FileSession::of(&hub).unwrap(), daemon)
    }

    const PATHS: [(&str, &str, &str); 3] = [
        ("a/b.txt", "a", "a/b.txt"),
        ("b.txt", "", "b.txt"),
        ("a/b/c", "a/b", "a/b/c"),
    ];

    #[test]
    fn a_path_splits_into_its_directory_and_joins_back() {
        for (path, directory, joined) in PATHS {
            assert_eq!(parent_of(path), directory, "{path}");
            let name = path.rsplit('/').next().unwrap();
            assert_eq!(join(directory, name), joined, "{path}");
        }
        let (idle, _) = state();
        let hub = Hub::new(idle, Arc::new(|| {}));
        assert!(FileSession::of(&hub).is_err(), "no workspace is followed");
    }

    #[tokio::test]
    async fn a_missing_document_does_not_exist_and_a_present_one_does() {
        let (session, daemon) = session();
        daemon.ok(
            "GET",
            "/v0/documents/content?workspace=hw&path=a.txt",
            r#"{"content":"x"}"#,
        );
        assert!(session.exists("a.txt").await.unwrap());
        assert!(!session.exists("b.txt").await.unwrap());
    }
}
