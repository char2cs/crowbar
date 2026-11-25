use editorview::DiskTake;
use gpui::App;
use protocol::{DocumentPath, PaneId};

use crate::panes::editor::document_source::{save_failed, saved_as_into};
use crate::panes::editor::editor_panes::load::arrival::Arrival;
use crate::panes::editor::editor_panes::view::view_events::listen;
use crate::panes::editor::editor_panes::{Adoption, EditorPanes, SaveAsPanes};
use crate::panes::editor::saved_file::SavedFile;
use crate::shell::inbox::request_queue::send;

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
        if let Some(document) = followed.document(cx) {
            document.update(cx, |document, cx| document.rebind(saved.path.as_str(), cx));
            document.update(cx, |document, cx| document.mark_saved(&saved.text, cx));
        }
        self.panes.borrow_mut().insert(file_pane, followed);
    }

    pub(crate) fn adopt(
        &self,
        (panes, saved): (SaveAsPanes<'_>, &SavedFile),
        cx: &mut App,
    ) -> Adoption {
        let SaveAsPanes { holder, untitled } = panes;
        let late = self
            .document_of(untitled, cx)
            .map(|document| document.read(cx).text(cx))
            .filter(|typed| *typed != saved.text);
        let Some(document) = self.document_of(holder, cx) else {
            return Adoption::Replaces(DiskTake::Kept);
        };
        let held = document.read(cx);
        let edited = held.is_dirty(cx) && held.text(cx) != saved.text;
        let took = document.update(cx, |document, cx| {
            let since = document.save_mark();
            document.take_disk_text((&saved.text, since), cx)
        });
        match (late, edited) {
            (Some(_), true) => Adoption::Stays(took),
            (Some(late), false) => {
                document.update(cx, |document, cx| document.take_page_text(&late, cx));
                Adoption::Replaces(took)
            },
            (None, _) => Adoption::Replaces(took),
        }
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

    use super::*;
    use crate::panes::editor::document_request::DocumentRequest;
    use crate::panes::editor::editor_panes::tests::{
        PANE, PATH, TEXT, landed, layout_with, panes_over, shown_in,
    };
    use crate::shell::inbox::shell_request::ShellRequest;
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
        panes.follow(&layout_with(json!({"untitled": true})), "");
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

    fn kind_once_saved_as(
        path: &str,
        cx: &mut TestAppContext,
    ) -> Option<panehost::PaneKind> {
        let documents = Rc::new(FakeDocuments::default());
        documents.loading("c1");
        let (panes, _heard) = panes_over(&documents);
        panes.follow(&layout_with(json!({"untitled": true})), "");
        let (_view, window) = shown_in(&panes, cx);
        let path = DocumentPath::new(path).unwrap();
        let saved = SavedFile {
            path,
            text: "hi".to_owned(),
        };
        let (pane, file_pane) = (PaneId::new(PANE).unwrap(), PaneId::new("f2").unwrap());
        window.update(|_, cx| panes.rebind((&pane, file_pane.clone()), &saved, cx));
        panes.kind_of(&file_pane)
    }

    #[gpui::test]
    fn an_untitled_pane_saved_as_markdown_shows_its_rich_view_as_develop_s_tab_does(
        cx: &mut TestAppContext
    ) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(editorview::init);
        let cases = [
            ("notes.md", panehost::PaneKind::Web),
            (PATH, panehost::PaneKind::Native),
        ];
        for (path, kind) in cases {
            assert_eq!(kind_once_saved_as(path, cx), Some(kind), "{path}");
        }
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
        panes.follow(&layout_with(json!({"path": PATH})), "");
        landed(&panes, &heard);
        let (view, window) = shown_in(&panes, cx);
        let holder = PaneId::new(PANE).unwrap();
        let saved = SavedFile {
            path: DocumentPath::new(PATH).unwrap(),
            text: "written\n".to_owned(),
        };
        let untitled = PaneId::new("u1").unwrap();
        let both = SaveAsPanes {
            holder: &holder,
            untitled: &untitled,
        };
        let took = window.update(|_, cx| panes.adopt((both, &saved), cx));
        assert_eq!(took, Adoption::Replaces(DiskTake::Taken));
        assert_eq!(
            view.read_with(window, editorview::EditorView::text),
            "written\n"
        );
        assert!(
            !view.read_with(window, |view, cx| view.document().read(cx).is_dirty(cx)),
            "clean"
        );
        window.update(|window, cx| window.focus(&view.focus_handle(cx), cx));
        let undo = window.update(|_, cx| crate::test_support::keys_bound_to("input::Undo", cx));
        window.simulate_keystrokes(&undo);
        assert_eq!(view.read_with(window, editorview::EditorView::text), TEXT);
    }

    #[test]
    fn a_file_is_held_by_any_pane_on_it_in_the_same_workspace() {
        let documents = Rc::new(FakeDocuments::default());
        documents.loading("c1");
        let (panes, _heard) = panes_over(&documents);
        panes.follow(&layout_with(json!({"path": PATH})), "");
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

    struct Both(Rc<EditorPanes>);

    impl gpui::Render for Both {
        fn render(
            &mut self,
            window: &mut gpui::Window,
            cx: &mut gpui::Context<'_, Self>,
        ) -> impl gpui::IntoElement {
            let drawn: Vec<_> = [PANE, UNTITLED]
                .iter()
                .filter_map(|pane| self.0.content(&PaneId::new(*pane).unwrap(), window, cx))
                .collect();
            gpui::ParentElement::children(gpui::div(), drawn)
        }
    }

    const UNTITLED: &str = "u1";

    fn with_untitled(mut layout: layout::Layout) -> layout::Layout {
        let instance = layout::PaneInstance {
            pane_id: PaneId::new(UNTITLED).unwrap(),
            type_id: protocol::PaneTypeId::new(crate::panes::kinds::core_panes::EDITOR).unwrap(),
            params: protocol::Params::default()
                .with("threadId", "c1")
                .and_then(|params| params.with("fileId", "u1"))
                .unwrap(),
            state: protocol::PaneState::new(json!({"untitled": true})).unwrap(),
        };
        let commands = [
            layout::Command::AddTab {
                id: layout::TabId(2),
                title: String::new(),
                space: layout::SpaceId::default(),
            },
            layout::Command::OpenPane {
                tab: layout::TabId(2),
                instance,
                placement: None,
            },
        ];
        for command in commands {
            layout.apply(command).unwrap();
        }
        layout
    }

    fn typed(
        view: &gpui::Entity<editorview::EditorView>,
        (window, text): (&mut gpui::VisualTestContext, &str),
    ) {
        window.update(|window, cx| window.focus(&view.focus_handle(cx), cx));
        window.simulate_input(text);
    }

    type Views = [gpui::Entity<editorview::EditorView>; 2];

    fn file_and_untitled_shown(
        cx: &mut TestAppContext
    ) -> (Rc<EditorPanes>, Views, &mut gpui::VisualTestContext) {
        let documents = Rc::new(FakeDocuments::default());
        documents.loading("c1");
        documents.holding(PATH, TEXT);
        let (panes, heard) = panes_over(&documents);
        panes.follow(&with_untitled(layout_with(json!({"path": PATH}))), "");
        landed(&panes, &heard);
        let drawn = Rc::clone(&panes);
        let (_root, window) = cx.add_window_view(|_, _| Both(drawn));
        window.run_until_parked();
        let views = [PANE, UNTITLED].map(|pane| {
            let panes = panes.panes.borrow();
            panes[&PaneId::new(pane).unwrap()].view().cloned().unwrap()
        });
        (panes, views, window)
    }

    #[gpui::test]
    fn text_typed_into_the_untitled_tab_during_the_write_stays_unsaved_in_the_file_pane(
        cx: &mut TestAppContext
    ) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(editorview::init);
        let mine = format!("mine {TEXT}");
        let cases = [
            (
                "",
                Adoption::Replaces(DiskTake::Taken),
                ("late ", "written\n"),
            ),
            (
                "mine ",
                Adoption::Stays(DiskTake::Conflicted),
                (mine.as_str(), TEXT),
            ),
        ];
        let saved = SavedFile {
            path: DocumentPath::new(PATH).unwrap(),
            text: "written\n".to_owned(),
        };
        let (holder_pane, untitled_pane) =
            (PaneId::new(PANE).unwrap(), PaneId::new(UNTITLED).unwrap());
        let both = SaveAsPanes {
            holder: &holder_pane,
            untitled: &untitled_pane,
        };
        for (holder_typing, expected, (text, saved_text)) in cases {
            let (panes, [holder, untitled], window) = file_and_untitled_shown(cx);
            typed(&untitled, (window, "late "));
            typed(&holder, (window, holder_typing));
            let adopted = window.update(|_, cx| panes.adopt((both, &saved), cx));
            assert_eq!(adopted, expected);
            assert_eq!(holder.read_with(window, editorview::EditorView::text), text);
            assert_eq!(
                holder.read_with(window, |view, cx| view.document().read(cx).saved_text()),
                saved_text
            );
        }
    }
}
