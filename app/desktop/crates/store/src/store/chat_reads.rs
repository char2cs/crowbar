//! What the shell reads about one chat.

use crate::Store;
use crate::live::ChatTerminal;

impl Store {
    /// The surface, runner, terminal session and phase of `chat`; `None` when it is not loaded.
    pub fn chat_terminal(
        &self,
        chat: &str,
    ) -> Option<ChatTerminal> {
        self.hub.read(|state| ChatTerminal::of(state, chat))
    }

    /// The SVG artwork of the provider `chat` runs on, to draw in the host's ink; `None` until
    /// the chat and its host's providers are known.
    pub fn chat_glyph(
        &self,
        chat: &str,
    ) -> Option<String> {
        self.hub
            .read(|state| crate::reads::chat_glyph::of(state, chat))
    }

    /// Whether `chat` has a branch to review: it owns its worktree, as a repository's own
    /// checkout or a branch does; a thread, a project home chat or an unknown chat has none.
    pub fn chat_reviews_branch(
        &self,
        chat: &str,
    ) -> bool {
        self.hub
            .read(|state| crate::reads::branch_review::of(state, chat))
    }

    /// Whether `chat`'s files are read-only: its workspace is locked, as develop's
    /// `useWorkspaceReadOnly`; an unknown chat's are not.
    pub fn chat_read_only(
        &self,
        chat: &str,
    ) -> bool {
        self.hub
            .read(|state| crate::reads::read_only::of(state, chat))
    }

    /// The project `chat` belongs to; `None` when it is not loaded or has none.
    pub fn chat_project(
        &self,
        chat: &str,
    ) -> Option<String> {
        self.hub
            .read(|state| crate::live::chat_project::of(state, chat))
    }

    /// The folder `chat` works in, on whichever computer holds it; `None` when it is not loaded.
    pub fn chat_root(
        &self,
        chat: &str,
    ) -> Option<std::path::PathBuf> {
        self.hub
            .read(|state| crate::live::workspace_root::of_chat(state, chat))
    }

    /// The name of the other computer `chat` runs on; `None` when it runs on this one.
    pub fn chat_remote_host(
        &self,
        chat: &str,
    ) -> Option<String> {
        self.hub
            .read(|state| crate::live::workspace_root::remote_host_of(state, chat))
    }

    /// The id of the host `chat` runs on; `None` when it is not loaded.
    pub fn chat_host(
        &self,
        chat: &str,
    ) -> Option<String> {
        self.hub
            .read(|state| crate::live::workspace_root::host_id_of(state, chat))
    }

    /// `chat` as a view tab draws it: its title, or "Untitled chat" provisionally, and the
    /// branch status of the workspace it owns; `None` when it is not loaded.
    pub fn chat_row(
        &self,
        chat: &str,
    ) -> Option<sidebar::Row> {
        self.hub.read(|state| {
            state
                .hosts
                .iter()
                .find_map(|slot| crate::rows::chat_member::of(&slot.store, chat))
        })
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use daemonapi::Frame;

    use super::*;
    use crate::test_support::{FakeDaemon, desk, quiet};

    fn local_store() -> Store {
        let transport: Arc<dyn protocol::DaemonTransport> = FakeDaemon::shared();
        let spec = crate::live::HostSpec {
            id: "local".into(),
            name: "This computer".into(),
            endpoint: None,
        };
        let wiring = crate::store::ports::Ports {
            desk: desk(),
            signal: quiet(),
            outcome: Arc::new(|_| ()),
        };
        Store::start(
            &tokio::runtime::Handle::current(),
            &transport,
            vec![spec],
            wiring,
        )
    }

    #[tokio::test]
    async fn a_loaded_chat_reads_its_project_and_its_tab_row() {
        let data = local_store();
        let frame = Frame::decode(
            r#"{"t":"entity","scope":"projects","kind":"chat","id":"c","v":1,"data":{"id":"c","projectId":"p","working":true}}"#,
        )
        .unwrap();
        data.hub.edit(|state| state.apply("local", frame));
        assert_eq!(data.chat_project("c").as_deref(), Some("p"));
        let row = data.chat_row("c").unwrap();
        assert_eq!(row.activity, sidebar::Activity::Working, "its turn runs");
        assert!(
            data.chat_row("gone").is_none(),
            "an unknown chat has no row"
        );
        assert!(data.chat_terminal("gone").is_none());
    }
}
