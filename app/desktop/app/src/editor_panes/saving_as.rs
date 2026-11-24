use editorview::DiskTake;
use gpui::{App, Window};
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
        saved.path.as_str().clone_into(&mut followed.target.label);
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

    pub(crate) fn adopt(
        &self,
        (holder, saved): (&PaneId, &SavedFile),
        window: &mut Window,
        cx: &mut App,
    ) -> DiskTake {
        let view = self
            .panes
            .borrow()
            .get(holder)
            .and_then(|followed| followed.view().cloned());
        let Some(view) = view else {
            return DiskTake::Kept;
        };
        view.update(cx, |view, cx| {
            let since = view.save_mark();
            view.take_disk_text((&saved.text, since), window, cx)
        })
    }

    pub(crate) fn holders(
        &self,
        (chat, path): (&str, &DocumentPath),
    ) -> Vec<PaneId> {
        let workspace = (self.chats.workspaces)(chat);
        let shares = |held: &str| {
            held == chat
                || workspace
                    .as_deref()
                    .is_some_and(|w| (self.chats.workspaces)(held).as_deref() == Some(w))
        };
        self.panes
            .borrow()
            .iter()
            .filter(|(_, followed)| followed.target.file.as_ref() == Some(path))
            .filter(|(_, followed)| shares(&followed.target.chat))
            .map(|(pane, _)| pane.clone())
            .collect()
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

    #[gpui::test]
    fn a_file_pane_adopting_a_save_as_shows_the_written_text_saved_and_keeps_its_undo(
        cx: &mut TestAppContext
    ) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(editorview::init);
        let documents = Rc::new(FakeDocuments::default());
        documents.loading("c1");
        documents.holding(PATH, TEXT);
        let (panes, heard) = panes_over(&documents);
        panes.follow(&layout_with(json!({"path": PATH})));
        landed(&panes, &heard);
        let (view, window) = shown_in(&panes, cx);
        let holder = PaneId::new(PANE).unwrap();
        let saved = SavedFile {
            path: DocumentPath::new(PATH).unwrap(),
            text: "written\n".to_owned(),
        };
        let took = window.update(|window, cx| panes.adopt((&holder, &saved), window, cx));
        assert_eq!(took, DiskTake::Taken);
        assert_eq!(
            view.read_with(window, editorview::EditorView::text),
            "written\n"
        );
        assert!(
            !view.read_with(window, editorview::EditorView::is_dirty),
            "clean"
        );
        window.update(|window, cx| window.focus(&view.focus_handle(cx), cx));
        window.simulate_keystrokes("ctrl-z");
        assert_eq!(view.read_with(window, editorview::EditorView::text), TEXT);
    }

    #[test]
    fn a_file_is_held_by_any_pane_on_it_in_the_same_workspace() {
        let documents = Rc::new(FakeDocuments::default());
        documents.loading("c1");
        let (panes, _heard) = panes_over(&documents);
        panes.follow(&layout_with(json!({"path": PATH})));
        let path = DocumentPath::new(PATH).unwrap();
        let other = DocumentPath::new("src/other.ts").unwrap();
        let holder = vec![PaneId::new(PANE).unwrap()];
        let cases = [
            (("c1", &path), holder.clone()),
            (("c2", &path), holder),
            (("c3", &path), vec![]),
            (("c2", &other), vec![]),
        ];
        for (asked, expected) in cases {
            assert_eq!(panes.holders(asked), expected, "{asked:?}");
        }
    }
}
