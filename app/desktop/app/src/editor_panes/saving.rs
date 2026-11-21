use gpui::App;
use protocol::PaneId;

use super::EditorPanes;
use super::followed::Followed;
use crate::document_source::{save_failed, saved_into};
use crate::request_queue::send;

impl EditorPanes {
    pub(crate) fn save(
        &self,
        pane: &PaneId,
        cx: &App,
    ) {
        let panes = self.panes.borrow();
        let Some(followed) = panes.get(pane) else {
            return;
        };
        let (Some(view), Some(file)) = (followed.view(), &followed.target.file) else {
            return;
        };
        let text = view.read(cx).text(cx);
        let done = saved_into(self.requests.clone(), pane.clone(), text.clone());
        let target = (followed.target.chat.as_str(), file);
        if let Err(error) = self.documents.write(target, &text, done) {
            send(&self.requests, save_failed(pane.clone(), &error));
        }
    }

    pub(crate) fn saved(
        &self,
        pane: &PaneId,
        text: &str,
        cx: &mut App,
    ) {
        let view = self
            .panes
            .borrow()
            .get(pane)
            .and_then(Followed::view)
            .cloned();
        if let Some(view) = view {
            view.update(cx, |view, cx| view.mark_saved(text, cx));
        }
    }

    pub(crate) fn autosave(
        &self,
        pane: &PaneId,
        cx: &App,
    ) {
        let dirty = self
            .panes
            .borrow()
            .get(pane)
            .and_then(Followed::view)
            .is_some_and(|view| view.read(cx).is_dirty(cx));
        if dirty {
            self.save(pane, cx);
        }
    }
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::{Focusable as _, TestAppContext};
    use serde_json::json;

    use super::super::tests::{PANE, PATH, TEXT, landed, layout_with, panes_over, shown_in};
    use super::*;
    use crate::document_request::DocumentRequest;
    use crate::editor_panes::Landing;
    use crate::shell_request::ShellRequest;
    use crate::test_support::FakeDocuments;

    #[gpui::test]
    fn a_save_writes_the_shown_text_and_tells_the_shell_it_reached_the_file(
        cx: &mut TestAppContext
    ) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(editorview::init);
        let documents = Rc::new(FakeDocuments::default());
        documents.loading("c1");
        documents.holding(PATH, TEXT);
        let (panes, heard) = panes_over(&documents);
        panes.follow(&layout_with(json!({"path": PATH})));
        assert_eq!(landed(&panes, &heard), Landing::Shown);
        let (_root, window) = cx.add_window_view(|_, _| gpui::Empty);
        let pane = PaneId::new(PANE).unwrap();
        window.update(|window, cx| panes.content(&pane, window, cx));
        window.update(|_, cx| panes.save(&pane, cx));
        assert_eq!(documents.writes(), [(PATH.to_owned(), TEXT.to_owned())]);
        let saved = ShellRequest::Document(DocumentRequest::Saved {
            pane,
            text: TEXT.to_owned(),
        });
        assert_eq!(heard.try_recv().unwrap(), saved);
    }

    #[gpui::test]
    fn an_autosave_writes_only_a_dirty_file(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(editorview::init);
        let cases = [
            (
                json!({"path": PATH}),
                "x",
                vec![(PATH.to_owned(), format!("x{TEXT}"))],
            ),
            (json!({"path": PATH}), "", vec![]),
            (json!({"untitled": true}), "x", vec![]),
        ];
        for (state, typed, expected) in cases {
            let documents = Rc::new(FakeDocuments::default());
            documents.loading("c1");
            documents.holding(PATH, TEXT);
            let (panes, heard) = panes_over(&documents);
            panes.follow(&layout_with(state.clone()));
            landed(&panes, &heard);
            let (view, window) = shown_in(&panes, cx);
            window.update(|window, cx| window.focus(&view.focus_handle(cx), cx));
            window.simulate_input(typed);
            let pane = PaneId::new(PANE).unwrap();
            window.update(|_, cx| panes.autosave(&pane, cx));
            assert_eq!(documents.writes(), expected, "{state}, typed {typed:?}");
        }
    }
}
