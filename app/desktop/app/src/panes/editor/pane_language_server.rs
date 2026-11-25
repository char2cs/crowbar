//! A file pane's language servers: the daemon's LSP routes for the pane's chat, through the store.

use std::rc::Rc;

use gpui::{App, AppContext as _, Task};
use serde_json::Value;

use crate::panes::editor::chat_servers::ChatServers;

pub(crate) struct PaneLanguageServer {
    servers: store::LanguageServers,
    chat: String,
}

impl PaneLanguageServer {
    pub(crate) fn per_chat(servers: store::LanguageServers) -> ChatServers {
        Rc::new(move |chat| Self::of(&servers, chat))
    }

    fn of(
        servers: &store::LanguageServers,
        chat: &str,
    ) -> Rc<dyn editorview::LanguageServer> {
        let servers = servers.clone();
        let chat = chat.to_owned();
        Rc::new(Self { servers, chat })
    }
}

impl editorview::LanguageServer for PaneLanguageServer {
    fn post(
        &self,
        verb: &'static str,
        body: Value,
        cx: &mut App,
    ) -> Task<Result<Value, editorview::LspError>> {
        let (sender, receiver) = async_channel::bounded(1);
        let done: store::FileDone<Value> = Box::new(move |answer| {
            let gone = sender.try_send(answer).is_err();
            tracing::debug!(verb, gone, "the language server answered");
        });
        self.servers.post((&self.chat, verb), body, done);
        cx.background_spawn(async move {
            let answer = receiver.recv().await;
            let answer = answer.map_err(|error| editorview::LspError(error.to_string()))?;
            answer.map_err(|error| editorview::LspError(error.to_string()))
        })
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use gpui::TestAppContext;
    use protocol::DaemonTransport;

    use super::*;

    fn data(runtime: &tokio::runtime::Runtime) -> store::Store {
        let transport: Arc<dyn DaemonTransport> =
            Arc::new(crate::test_support::NoDaemon::timing_out());
        let registry = Arc::new(transport::HostRegistry::in_memory());
        let tokens = Arc::new(transport::MemoryTokenStore::default());
        let desk = store::HostDesk {
            book: Arc::new(
                crate::daemon::daemon_link::transport_book::TransportBook::new(registry, tokens),
            ),
            notes: Arc::new(
                store::HostNotes::open("/nonexistent-crowbar-home/n.json".into()).unwrap(),
            ),
        };
        let wiring = store::Ports {
            desk,
            signal: Arc::new(|| {}),
            outcome: Arc::new(|_| {}),
        };
        store::Store::start(runtime.handle(), &transport, Vec::new(), wiring)
    }

    #[gpui::test]
    fn a_pane_of_a_chat_that_is_not_loaded_hears_no_server(cx: &mut TestAppContext) {
        let runtime = tokio::runtime::Builder::new_current_thread()
            .build()
            .unwrap();
        let server = PaneLanguageServer::per_chat(data(&runtime).language_servers())("c1");
        let body = serde_json::json!({"path": "a.ts"});
        let asked = cx.update(|cx| server.post("hover", body, cx));
        let answer = cx.foreground_executor().block_test(asked);
        assert_eq!(answer.ok(), Some(Value::Null));
    }
}
