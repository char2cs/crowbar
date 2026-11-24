//! One view's document at its language server, as develop's `document-lsp.ts` and
//! `lsp-client.ts` keep it: didOpen with the text, a full-text didChange once edits pause (sent
//! early by any request), didSave and didClose.

use std::cell::RefCell;
use std::rc::Rc;
use std::time::Duration;

use gpui::{App, AppContext as _, Task, WeakEntity};
use gpui_kit::component::input::EditorState;
use serde_json::{Map, Value, json};

use crate::{LanguageServer, LspError};

const CHANGE_DEBOUNCE: Duration = Duration::from_millis(400);
const UNTITLED: &str = "untitled:";
const HOST_ROOT: char = '/';
const PATH: &str = "path";

pub(crate) struct LspLink {
    server: Rc<dyn LanguageServer>,
    state: WeakEntity<EditorState>,
    path: RefCell<Option<String>>,
    pending: RefCell<Option<Task<()>>>,
}

impl LspLink {
    pub(crate) fn new(
        server: Rc<dyn LanguageServer>,
        state: WeakEntity<EditorState>,
    ) -> Rc<Self> {
        Rc::new(Self {
            server,
            state,
            path: RefCell::default(),
            pending: RefCell::default(),
        })
    }

    pub(crate) fn open(
        &self,
        path: &str,
        cx: &mut App,
    ) {
        self.close(cx);
        if path.starts_with(UNTITLED) || path.starts_with(HOST_ROOT) {
            return;
        }
        let Some(text) = self.text(cx) else {
            return;
        };
        let language = editor::Language::of_path(path).id();
        let body = json!({ PATH: path, "languageId": language, "text": text });
        *self.path.borrow_mut() = Some(path.to_owned());
        self.send("didOpen", body, cx).detach();
    }

    pub(crate) fn close(
        &self,
        cx: &mut App,
    ) {
        self.pending.borrow_mut().take();
        let Some(path) = self.path.borrow_mut().take() else {
            return;
        };
        self.send("didClose", json!({ PATH: path }), cx).detach();
    }

    pub(crate) fn saved(
        &self,
        cx: &mut App,
    ) {
        let Some(path) = self.path.borrow().clone() else {
            return;
        };
        self.send("didSave", json!({ PATH: path }), cx).detach();
    }

    pub(crate) fn changed(
        self: &Rc<Self>,
        cx: &mut App,
    ) {
        if self.path.borrow().is_none() {
            return;
        }
        let link = Rc::clone(self);
        let timer = cx.background_executor().timer(CHANGE_DEBOUNCE);
        let pending = cx.spawn(async move |cx| {
            timer.await;
            cx.update(|cx| link.flush(cx).detach());
        });
        *self.pending.borrow_mut() = Some(pending);
    }

    pub(crate) fn request(
        &self,
        verb: &'static str,
        fields: Map<String, Value>,
        cx: &mut App,
    ) -> Task<Result<Value, LspError>> {
        let Some(path) = self.path.borrow().clone() else {
            return Task::ready(Ok(Value::Null));
        };
        let flushed = self.flush(cx);
        let mut body = fields;
        body.insert(PATH.to_owned(), Value::String(path));
        let server = Rc::clone(&self.server);
        cx.spawn(async move |cx| {
            flushed.await;
            cx.update(|cx| server.post(verb, Value::Object(body), cx))
                .await
        })
    }

    fn flush(
        &self,
        cx: &mut App,
    ) -> Task<()> {
        if self.pending.borrow_mut().take().is_none() {
            return Task::ready(());
        }
        let (Some(path), Some(text)) = (self.path.borrow().clone(), self.text(cx)) else {
            return Task::ready(());
        };
        let sent = self.send("didChange", json!({ PATH: path, "text": text }), cx);
        cx.background_spawn(async move {
            sent.await;
        })
    }

    fn send(
        &self,
        verb: &'static str,
        body: Value,
        cx: &mut App,
    ) -> Task<()> {
        let sent = self.server.post(verb, body, cx);
        cx.background_spawn(async move { untaken(verb, sent.await) })
    }

    fn text(
        &self,
        cx: &App,
    ) -> Option<String> {
        let state = self.state.upgrade()?;
        Some(state.read(cx).value().to_string())
    }
}

fn untaken(
    verb: &'static str,
    sent: Result<Value, LspError>,
) {
    if let Err(error) = sent {
        tracing::warn!(%error, verb, "the language server did not take the document");
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use gpui::TestAppContext;

    use crate::editor_view::test_support::served;

    #[gpui::test]
    fn untitled_and_host_documents_have_no_server_to_tell(cx: &mut TestAppContext) {
        for path in ["untitled:Untitled-1", "/etc/hostname"] {
            let (_view, window, server) = served((path, ""), cx);
            window.simulate_input("x");
            window.executor().advance_clock(Duration::from_secs(1));
            window.run_until_parked();
            assert_eq!(server.verbs(), Vec::<&str>::new(), "{path}");
        }
    }
}
