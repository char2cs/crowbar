use std::collections::BTreeMap;

use editorview::{Document, EditorView};
use gpui::{App, AppContext as _, Entity, Window};
use protocol::PaneId;

use super::followed::Followed;
use crate::panes::editor::document_request::DocumentRequest;
use crate::panes::editor::editor_panes::view::keyboard::Keyboard;
use crate::panes::editor::editor_panes::view::view_events::listen;
use crate::panes::editor::editor_panes::{EditorPanes, EditorTarget, PanePhase};
use crate::shell::inbox::request_queue::send;
use crate::shell::inbox::shell_request::ShellRequest;

impl EditorPanes {
    pub(crate) fn arriving(
        &self,
        target: EditorTarget,
        seat: (Keyboard, editorview::Access),
        panes: &BTreeMap<PaneId, Followed>,
    ) -> Followed {
        let server = (self.chats.servers)(&target.chat);
        let shown = self.is_shown(&target, panes);
        let mut followed = Followed::arrive(target, seat, server);
        if shown {
            followed.phase = PanePhase::Joining;
        }
        followed
    }

    fn is_shown(
        &self,
        target: &EditorTarget,
        panes: &BTreeMap<PaneId, Followed>,
    ) -> bool {
        let Some(uri) = self.document_uri(target) else {
            return false;
        };
        panes.values().any(|followed| {
            matches!(followed.phase, PanePhase::Shown(_))
                && self.document_uri(&followed.target).as_ref() == Some(&uri)
        })
    }

    pub(crate) fn leads(
        &self,
        pane: &PaneId,
        cx: &App,
    ) -> bool {
        let panes = self.panes.borrow();
        let Some(document) = panes.get(pane).and_then(|followed| followed.document(cx)) else {
            return false;
        };
        panes
            .iter()
            .find(|(_, followed)| followed.document(cx).as_ref() == Some(&document))
            .is_some_and(|(first, _)| first == pane)
    }

    pub(crate) fn join(
        &self,
        pane: &PaneId,
        window: &mut Window,
        cx: &mut App,
    ) {
        let joining = self
            .panes
            .borrow()
            .get(pane)
            .filter(|followed| joins(followed))
            .map(|followed| followed.target.clone());
        let Some(target) = joining else {
            return;
        };
        let document = self.open_document(&target, cx);
        let mut panes = self.panes.borrow_mut();
        let Some(followed) = panes.get_mut(pane) else {
            return;
        };
        match document {
            Some(document) => {
                drop(panes);
                self.seat((pane, document), window, cx);
            },
            None if matches!(followed.phase, PanePhase::Joining) => {
                followed.phase = PanePhase::Waiting;
                drop(panes);
                self.read_waiting();
            },
            None => {},
        }
    }

    fn seat(
        &self,
        (pane, document): (&PaneId, Entity<Document>),
        window: &mut Window,
        cx: &mut App,
    ) {
        let dirty = document.read(cx).is_dirty(cx);
        let mut panes = self.panes.borrow_mut();
        let Some(followed) = panes.get_mut(pane) else {
            return;
        };
        let opening = followed.opening();
        let view = cx.new(|cx| EditorView::new(document, opening, window, cx));
        followed.told = Some(listen(&view, (pane, &self.requests), cx));
        followed.phase = PanePhase::Shown(view);
        if !dirty {
            return;
        }
        let dirty = DocumentRequest::Dirty {
            pane: pane.clone(),
            dirty,
        };
        send(&self.requests, ShellRequest::Document(dirty));
    }

    pub(crate) fn open_document(
        &self,
        target: &EditorTarget,
        cx: &App,
    ) -> Option<Entity<Document>> {
        let uri = self.uri_of(target)?;
        self.panes
            .borrow()
            .values()
            .filter(|followed| self.document_uri(&followed.target).as_ref() == Some(&uri))
            .find_map(|followed| followed.document(cx))
    }

    fn document_uri(
        &self,
        target: &EditorTarget,
    ) -> Option<String> {
        target.preview.is_none().then(|| self.uri_of(target))?
    }
}

fn joins(followed: &Followed) -> bool {
    matches!(followed.phase, PanePhase::Joining | PanePhase::Read(_))
        && followed.target.file.is_some()
        && followed.target.preview.is_none()
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::{Focusable as _, IntoElement as _, TestAppContext, VisualTestContext};
    use serde_json::json;

    use super::*;
    use crate::panes::editor::document_request::DocumentRequest;
    use crate::panes::editor::editor_panes::tests::{
        PANE, PATH, TEXT, landed, layout_in, panes_over, two_panes_on, with_pane,
    };
    use crate::shell::inbox::shell_request::ShellRequest;
    use crate::test_support::FakeDocuments;

    const SECOND: &str = "f2";

    struct Two(Rc<EditorPanes>);

    impl gpui::Render for Two {
        fn render(
            &mut self,
            window: &mut Window,
            cx: &mut gpui::Context<'_, Self>,
        ) -> impl gpui::IntoElement {
            let drawn = [PANE, SECOND].map(|pane| drawn(&self.0, pane, (window, cx)));
            gpui::ParentElement::children(gpui::div(), drawn)
        }
    }

    fn drawn(
        panes: &EditorPanes,
        pane: &str,
        (window, cx): (&mut Window, &mut App),
    ) -> gpui::AnyElement {
        panes
            .content(&PaneId::new(pane).unwrap(), window, cx)
            .unwrap_or_else(|| gpui::Empty.into_any_element())
    }

    struct Shared<'cx> {
        panes: Rc<EditorPanes>,
        documents: Rc<FakeDocuments>,
        heard: async_channel::Receiver<ShellRequest>,
        window: &'cx mut VisualTestContext,
    }

    fn shared(cx: &mut TestAppContext) -> Shared<'_> {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(editorview::init);
        let documents = Rc::new(FakeDocuments::default());
        documents.loading("c1");
        documents.loading("c2");
        documents.holding(PATH, TEXT);
        let (panes, heard) = panes_over(&documents);
        panes.follow(&layout_in("c1", json!({"path": PATH})), "");
        landed(&panes, &heard);
        let drawn = Rc::clone(&panes);
        let (root, window) = cx.add_window_view(|_, _| Two(drawn));
        window.run_until_parked();
        focus(&panes, PANE, window);
        window.simulate_input("x");
        panes.follow(&two_panes_on(PATH), "");
        root.update(window, |_, cx| cx.notify());
        window.run_until_parked();
        Shared {
            panes,
            documents,
            heard,
            window,
        }
    }

    fn view_of(
        panes: &EditorPanes,
        pane: &str,
    ) -> Entity<EditorView> {
        let panes = panes.panes.borrow();
        let followed = &panes[&PaneId::new(pane).unwrap()];
        followed.view().cloned().expect("the pane shows its file")
    }

    fn focus(
        panes: &EditorPanes,
        pane: &str,
        window: &mut VisualTestContext,
    ) {
        let view = view_of(panes, pane);
        window.update(|window, cx| window.focus(&view.focus_handle(cx), cx));
    }

    fn text(
        panes: &EditorPanes,
        pane: &str,
        window: &mut VisualTestContext,
    ) -> String {
        let view = view_of(panes, pane);
        window.read(|cx| view.read(cx).text(cx))
    }

    fn dirty_told(heard: &async_channel::Receiver<ShellRequest>) -> Vec<(String, bool)> {
        std::iter::from_fn(|| heard.try_recv().ok())
            .filter_map(|request| match request {
                ShellRequest::Document(DocumentRequest::Dirty { pane, dirty }) => {
                    Some((pane.to_string(), dirty))
                },
                _ => None,
            })
            .collect()
    }

    #[gpui::test]
    fn a_second_pane_on_an_open_file_shows_its_unsaved_text_without_reading(
        cx: &mut TestAppContext
    ) {
        let Shared {
            panes,
            documents,
            heard,
            window,
        } = shared(cx);
        assert_eq!(documents.reads(), [PATH], "read once");
        let opened = documents.servers().told();
        let opened = opened.iter().filter(|(_, verb, _)| *verb == "didOpen");
        assert_eq!(opened.count(), 1, "one didOpen for the document");
        assert_eq!(text(&panes, SECOND, window), format!("x{TEXT}"));
        let told = dirty_told(&heard);
        assert!(told.contains(&(SECOND.to_owned(), true)), "{told:?}");
    }

    #[gpui::test]
    fn typing_in_one_pane_shows_in_the_other_and_saving_from_either_cleans_both(
        cx: &mut TestAppContext
    ) {
        let Shared {
            panes,
            heard,
            window,
            ..
        } = shared(cx);
        dirty_told(&heard);
        focus(&panes, SECOND, window);
        window.simulate_input("y");
        let typed = format!("yx{TEXT}");
        assert_eq!(
            text(&panes, PANE, window),
            typed,
            "f2 types at its own caret"
        );
        window.update(|_, cx| panes.saved(&PaneId::new(SECOND).unwrap(), &typed, cx));
        window.run_until_parked();
        let cleaned = [(PANE.to_owned(), false), (SECOND.to_owned(), false)];
        assert_eq!(dirty_told(&heard), cleaned);
    }

    #[gpui::test]
    fn a_change_on_disk_reads_the_file_once_and_pauses_act_once(cx: &mut TestAppContext) {
        let Shared {
            panes,
            documents,
            window,
            ..
        } = shared(cx);
        window.update(|_, cx| panes.disk_changed(("w1", Some(PATH)), cx));
        assert_eq!(documents.reads(), [PATH, PATH], "one read for the change");
        let leads = window.update(|_, cx| {
            [PANE, SECOND].map(|pane| panes.leads(&PaneId::new(pane).unwrap(), cx))
        });
        assert_eq!(leads, [true, false]);
    }

    #[gpui::test]
    fn a_preview_follows_the_open_documents_text_whichever_chat_typed(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(editorview::init);
        let documents = Rc::new(FakeDocuments::default());
        documents.loading("c1");
        documents.loading("c2");
        documents.holding(PATH, TEXT);
        let (panes, heard) = panes_over(&documents);
        let preview = json!({"path": PATH, "preview": "html"});
        let layout = with_pane(
            layout_in("c2", json!({"path": PATH})),
            ("f3", "c1"),
            preview,
        );
        panes.follow(&layout, "");
        landed(&panes, &heard);
        landed(&panes, &heard);
        let drawn = Rc::clone(&panes);
        let (_root, window) = cx.add_window_view(|_, _| Two(drawn));
        window.run_until_parked();
        focus(&panes, PANE, window);
        window.simulate_input("x");
        let shown = PaneId::new("f3").unwrap();
        let page = window.read(|cx| panes.page_text(&shown, cx));
        assert_eq!(page, Some(format!("x{TEXT}")));
        let source = PaneId::new(PANE).unwrap();
        assert_eq!(panes.previews_of(&source), [shown]);
    }
}
