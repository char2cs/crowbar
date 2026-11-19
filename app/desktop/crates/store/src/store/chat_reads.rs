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

    /// The project `chat` belongs to; `None` when it is not loaded or has none.
    pub fn chat_project(
        &self,
        chat: &str,
    ) -> Option<String> {
        self.hub
            .read(|state| crate::live::chat_project::of(state, chat))
    }

    /// Whether `chat` has a turn running; `false` when it is not loaded.
    pub fn chat_working(
        &self,
        chat: &str,
    ) -> bool {
        self.hub
            .read(|state| crate::live::chat_working::of(state, chat))
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
    async fn a_loaded_chat_reads_its_project_and_whether_it_works() {
        let data = local_store();
        let frame = Frame::decode(
            r#"{"t":"entity","scope":"projects","kind":"chat","id":"c","v":1,"data":{"id":"c","projectId":"p","working":true}}"#,
        )
        .unwrap();
        data.hub.edit(|state| state.apply("local", frame));
        assert_eq!(data.chat_project("c").as_deref(), Some("p"));
        assert!(data.chat_working("c"), "its turn runs");
        assert!(!data.chat_working("gone"), "an unknown chat is idle");
        assert!(data.chat_terminal("gone").is_none());
    }
}
