use gpui::App;
use protocol::{DocumentPath, PaneId};

use super::EditorPanes;
use super::arrival::Arrival;
use super::followed::listen;
use crate::document_source::{save_failed, saved_as_into};
use crate::request_queue::send;
use crate::saved_file::SavedFile;

impl EditorPanes {
    pub(crate) fn untitled_chat(
        &self,
        pane: &PaneId,
    ) -> Option<String> {
        let panes = self.panes.borrow();
        let followed = panes.get(pane)?;
        let untitled = followed.target.file.is_none();
        untitled.then(|| followed.target.chat.clone())
    }

    pub(crate) fn save_as(
        &self,
        pane: &PaneId,
        path: &DocumentPath,
        cx: &App,
    ) {
        let panes = self.panes.borrow();
        let Some(followed) = panes.get(pane) else {
            return;
        };
        let Some(view) = followed.view() else {
            return;
        };
        let text = view.read(cx).text(cx);
        let saved = SavedFile {
            path: path.clone(),
            text: text.clone(),
        };
        let done = saved_as_into(self.requests.clone(), pane.clone(), saved);
        if let Err(error) = self
            .documents
            .write((&followed.target.chat, path), &text, done)
        {
            send(&self.requests, save_failed(pane.clone(), &error));
        }
    }

    pub(crate) fn rebind(
        &self,
        (untitled, file_pane): (&PaneId, PaneId),
        saved: &SavedFile,
        cx: &mut App,
    ) {
        let Some(mut followed) = self.panes.borrow_mut().remove(untitled) else {
            return;
        };
        followed.target.file = Some(saved.path.clone());
        followed.target.arrival = Arrival::Restored;
        followed.told = followed
            .view()
            .map(|view| listen(view, (&file_pane, &self.requests), cx));
        if let Some(view) = followed.view().cloned() {
            view.update(cx, |view, cx| view.rebind(saved.path.as_str(), cx));
            view.update(cx, |view, cx| view.mark_saved(&saved.text, cx));
        }
        self.panes.borrow_mut().insert(file_pane, followed);
    }

    pub(crate) fn forget(
        &self,
        pane: &PaneId,
    ) {
        self.panes.borrow_mut().remove(pane);
    }
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::{Focusable as _, TestAppContext};
    use serde_json::json;

    use super::super::tests::{PANE, PATH, layout_with, panes_over, shown_in};
    use super::*;
    use crate::document_request::DocumentRequest;
    use crate::shell_request::ShellRequest;
    use crate::test_support::FakeDocuments;

    fn saved_as_told(
        heard: &async_channel::Receiver<ShellRequest>,
        (pane, saved): (&PaneId, &SavedFile),
    ) -> bool {
        let expected = ShellRequest::Document(DocumentRequest::SavedAs {
            pane: pane.clone(),
            saved: saved.clone(),
        });
        std::iter::from_fn(|| heard.try_recv().ok()).any(|told| told == expected)
    }

    #[gpui::test]
    fn an_untitled_pane_saved_as_a_file_writes_its_text_and_then_shows_that_file_clean(
        cx: &mut TestAppContext
    ) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(editorview::init);
        let documents = Rc::new(FakeDocuments::default());
        documents.loading("c1");
        let (panes, heard) = panes_over(&documents);
        panes.follow(&layout_with(json!({"untitled": true})));
        let (view, window) = shown_in(&panes, cx);
        window.update(|window, cx| window.focus(&view.focus_handle(cx), cx));
        window.simulate_input("hi");
        let pane = PaneId::new(PANE).unwrap();
        assert_eq!(panes.untitled_chat(&pane).as_deref(), Some("c1"));
        let path = DocumentPath::new(PATH).unwrap();
        window.update(|_, cx| panes.save_as(&pane, &path, cx));
        assert_eq!(documents.writes(), [(PATH.to_owned(), "hi".to_owned())]);
        let saved = SavedFile {
            path,
            text: "hi".to_owned(),
        };
        assert!(
            saved_as_told(&heard, (&pane, &saved)),
            "told it was saved as"
        );
        let file_pane = PaneId::new("f2").unwrap();
        window.update(|_, cx| panes.rebind((&pane, file_pane.clone()), &saved, cx));
        assert!(
            !panes.contains(&pane) && panes.contains(&file_pane),
            "rebound"
        );
        assert_eq!(panes.untitled_chat(&file_pane), None, "a file now");
        let clean = ShellRequest::Document(DocumentRequest::Dirty {
            pane: file_pane,
            dirty: false,
        });
        assert_eq!(
            heard.try_recv().ok(),
            Some(clean),
            "clean, told as the file pane"
        );
    }
}
