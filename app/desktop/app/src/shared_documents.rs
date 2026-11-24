//! One didOpen and one didClose per document whatever panes show it, as develop's
//! `document-lsp.ts` keeps them per document, not per pane.

use std::cell::RefCell;
use std::collections::BTreeMap;
use std::rc::Rc;

use gpui::{App, Task};
use serde_json::Value;

use crate::chat_servers::ChatServers;

const DID_OPEN: &str = "didOpen";
const DID_CLOSE: &str = "didClose";
const PATH: &str = "path";

type Showing = Rc<RefCell<BTreeMap<(String, String), usize>>>;

pub(crate) struct SharedDocuments {
    server: Rc<dyn editorview::LanguageServer>,
    chat: String,
    showing: Showing,
}

impl SharedDocuments {
    pub(crate) fn per_chat(servers: ChatServers) -> ChatServers {
        let showing = Showing::default();
        Rc::new(move |chat| Self::of((servers(chat), chat), &showing))
    }

    fn of(
        (server, chat): (Rc<dyn editorview::LanguageServer>, &str),
        showing: &Showing,
    ) -> Rc<dyn editorview::LanguageServer> {
        let chat = chat.to_owned();
        let showing = Rc::clone(showing);
        Rc::new(Self {
            server,
            chat,
            showing,
        })
    }

    fn passes(
        &self,
        verb: &str,
        body: &Value,
    ) -> bool {
        let Some(path) = body.get(PATH).and_then(Value::as_str) else {
            return true;
        };
        let key = (self.chat.clone(), path.to_owned());
        let mut showing = self.showing.borrow_mut();
        let panes = showing.entry(key.clone()).or_default();
        match verb {
            DID_OPEN => {
                *panes += 1;
                *panes == 1
            },
            DID_CLOSE => {
                *panes = panes.saturating_sub(1);
                let last = *panes == 0;
                showing.retain(|_, panes| *panes > 0);
                last
            },
            _ => true,
        }
    }
}

impl editorview::LanguageServer for SharedDocuments {
    fn post(
        &self,
        verb: &'static str,
        body: Value,
        cx: &mut App,
    ) -> Task<Result<Value, editorview::LspError>> {
        if !self.passes(verb, &body) {
            return Task::ready(Ok(Value::Null));
        }
        self.server.post(verb, body, cx)
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use serde_json::json;

    use super::*;
    use crate::test_support::FakeServers;

    #[gpui::test]
    fn two_panes_on_one_document_open_it_once_and_close_it_with_the_last(cx: &mut TestAppContext) {
        let fake = FakeServers::default();
        let servers = SharedDocuments::per_chat(fake.servers());
        let (first, second, other) = (servers("c1"), servers("c1"), servers("c2"));
        let told = [
            (&first, DID_OPEN),
            (&second, DID_OPEN),
            (&other, DID_OPEN),
            (&first, DID_CLOSE),
            (&second, DID_CLOSE),
        ];
        for (server, verb) in told {
            let body = json!({"path": "src/a.ts"});
            cx.update(|cx| server.post(verb, body, cx)).detach();
        }
        let told: Vec<(String, &str)> = fake
            .told()
            .into_iter()
            .map(|(chat, verb, _)| (chat, verb))
            .collect();
        let expected = [("c1", DID_OPEN), ("c2", DID_OPEN), ("c1", DID_CLOSE)];
        let expected: Vec<(String, &str)> = expected
            .iter()
            .map(|(chat, verb)| ((*chat).to_owned(), *verb))
            .collect();
        assert_eq!(told, expected);
    }
}
