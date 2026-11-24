use gpui::{App, Window};
use panehost::PaneKind;
use protocol::{DocumentPath, PaneId, ShellFrame};

use super::followed::Followed;
use super::pane_phase::PanePhase;
use super::pane_view::PaneView;
use super::{EditorPanes, Keyboard};

impl EditorPanes {
    pub(crate) fn kind_of(
        &self,
        pane: &PaneId,
    ) -> Option<PaneKind> {
        let panes = self.panes.borrow();
        let followed = panes.get(pane)?;
        if self.draws_natively(followed) {
            return Some(PaneKind::Native);
        }
        Some(PaneKind::Web)
    }

    pub(crate) fn draws(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.kind_of(pane) == Some(PaneKind::Native)
    }

    pub(crate) fn show(
        &self,
        pane: &PaneId,
        view: PaneView,
    ) -> bool {
        let mut panes = self.panes.borrow_mut();
        let Some(followed) = panes.get_mut(pane) else {
            return false;
        };
        let shown = self.view_of(followed);
        let allowed = match view {
            PaneView::Source => shown == PaneView::Rich,
            PaneView::Rich => shown == PaneView::Source && Self::has_rich_view(followed),
            PaneView::Preview => false,
        };
        let Some(uri) = self.uri_of(&followed.target).filter(|_| allowed) else {
            return false;
        };
        self.source_shown.show(uri, view);
        if view == PaneView::Source {
            followed.keyboard = Keyboard::Take;
        }
        true
    }

    pub(crate) fn prepare(
        &self,
        pane: &PaneId,
        window: &mut Window,
        cx: &mut App,
    ) -> bool {
        self.join(pane, window, cx);
        let mut panes = self.panes.borrow_mut();
        let Some(followed) = panes
            .get_mut(pane)
            .filter(|f| self.view_of(f) == PaneView::Rich)
        else {
            return false;
        };
        followed.materialize((pane, &self.requests), window, cx);
        let Some(view) = followed.view() else {
            return false;
        };
        let text = view.read(cx).text(cx);
        if protocol::encode(&ShellFrame::DocumentText { text }).is_ok() {
            return false;
        }
        let Some(uri) = self.uri_of(&followed.target) else {
            return false;
        };
        self.source_shown.show(uri, PaneView::Source);
        true
    }

    pub(crate) fn take_page_text(
        &self,
        (pane, text): (&PaneId, &str),
        window: &mut Window,
        cx: &mut App,
    ) {
        if self.prepare(pane, window, cx) {
            return;
        }
        if let Some(document) = self.document_of(pane, cx) {
            document.update(cx, |document, cx| document.take_page_text(text, cx));
        }
    }

    pub(crate) fn page_text(
        &self,
        pane: &PaneId,
        cx: &App,
    ) -> Option<String> {
        let panes = self.panes.borrow();
        let followed = panes.get(pane)?;
        match self.view_of(followed) {
            PaneView::Source => None,
            PaneView::Rich => Some(followed.view()?.read(cx).text(cx)),
            PaneView::Preview => {
                let live = self.open_document(&followed.target, cx);
                let live = live.map(|document| document.read(cx).text(cx));
                live.or_else(|| landed_text(followed))
            },
        }
    }

    pub(crate) fn previews_of(
        &self,
        pane: &PaneId,
    ) -> Vec<PaneId> {
        let panes = self.panes.borrow();
        let Some(source) = panes.get(pane) else {
            return Vec::new();
        };
        let uri = self.uri_of(&source.target);
        panes
            .iter()
            .filter(|(id, _)| *id != pane)
            .filter(|(_, other)| self.view_of(other) != PaneView::Source)
            .filter(|(_, other)| {
                other.shows(&source.target) || uri.is_some() && self.uri_of(&other.target) == uri
            })
            .map(|(id, _)| id.clone())
            .collect()
    }

    pub(crate) fn preview_of(
        &self,
        pane: &PaneId,
    ) -> Option<(String, DocumentPath, editor::Preview)> {
        let panes = self.panes.borrow();
        let target = &panes.get(pane)?.target;
        let file = target.file.clone()?;
        let preview = editor::Preview::of_path(file.as_str())?;
        Some((target.chat.clone(), file, preview))
    }

    fn has_rich_view(followed: &Followed) -> bool {
        followed.target.view() == PaneView::Rich
    }

    pub(super) fn view_of(
        &self,
        followed: &Followed,
    ) -> PaneView {
        let view = followed.target.view();
        let uri = self.uri_of(&followed.target);
        let source = uri.is_some_and(|uri| self.source_shown.contains(&uri));
        if view == PaneView::Rich && source {
            return PaneView::Source;
        }
        view
    }

    pub(super) fn draws_natively(
        &self,
        followed: &Followed,
    ) -> bool {
        self.view_of(followed) == PaneView::Source || followed.phase.says_why()
    }
}

fn landed_text(followed: &Followed) -> Option<String> {
    match &followed.phase {
        PanePhase::Read(landed) => landed.text(),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::{Focusable as _, TestAppContext};
    use serde_json::json;

    use super::super::tests::{PANE, landed, layout_with, panes_over, two_panes_on};
    use super::*;
    use crate::document_request::DocumentRequest;
    use crate::shell_request::ShellRequest;
    use crate::test_support::FakeDocuments;

    const README: &str = "README.md";
    const TITLE: &str = "# Title\n";

    fn kind(
        panes: &EditorPanes,
        pane: &PaneId,
    ) -> Option<PaneKind> {
        panes.kind_of(pane)
    }

    #[test]
    fn markdown_opens_in_its_page_other_files_in_the_native_editor() {
        let cases = [
            (json!({"path": README}), Some(PaneKind::Web)),
            (json!({"path": "docs/a.markdown"}), Some(PaneKind::Web)),
            (json!({"path": "site/index.html"}), Some(PaneKind::Native)),
            (json!({"path": "src/pricing.ts"}), Some(PaneKind::Native)),
            (json!({"untitled": true}), Some(PaneKind::Native)),
            (
                json!({"path": README, "preview": "markdown"}),
                Some(PaneKind::Web),
            ),
        ];
        let pane = PaneId::new(PANE).unwrap();
        for (state, expected) in cases {
            let documents = Rc::new(FakeDocuments::default());
            let (panes, _heard) = panes_over(&documents);
            panes.follow(&layout_with(state.clone()), "");
            assert_eq!(kind(&panes, &pane), expected, "{state}");
        }
    }

    #[test]
    fn a_restored_markdown_file_that_is_gone_says_why_natively_until_it_reads() {
        let documents = Rc::new(FakeDocuments::default());
        documents.loading("c1");
        let (panes, heard) = panes_over(&documents);
        panes.follow(&layout_with(json!({"path": README})), "");
        landed(&panes, &heard);
        let pane = PaneId::new(PANE).unwrap();
        assert_eq!(kind(&panes, &pane), Some(PaneKind::Native), "gone");
        documents.holding(README, TITLE);
        panes.retry(&pane);
        assert_eq!(kind(&panes, &pane), Some(PaneKind::Native), "trying again");
        landed(&panes, &heard);
        assert_eq!(kind(&panes, &pane), Some(PaneKind::Web), "read");
    }

    fn rich_pane(
        cx: &mut TestAppContext
    ) -> (
        Rc<EditorPanes>,
        async_channel::Receiver<ShellRequest>,
        &mut gpui::VisualTestContext,
    ) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(editorview::init);
        let documents = Rc::new(FakeDocuments::default());
        documents.loading("c1");
        documents.holding(README, TITLE);
        let (panes, heard) = panes_over(&documents);
        panes.follow(&layout_with(json!({"path": README})), "");
        landed(&panes, &heard);
        let pane = PaneId::new(PANE).unwrap();
        let (_root, window) = cx.add_window_view(|_, _| gpui::Empty);
        let flipped = window.update(|window, cx| panes.prepare(&pane, window, cx));
        assert!(!flipped, "a small file stays rich");
        (panes, heard, window)
    }

    fn text_of(
        panes: &EditorPanes,
        window: &mut gpui::VisualTestContext,
    ) -> Option<String> {
        let pane = PaneId::new(PANE).unwrap();
        window.read(|cx| panes.page_text(&pane, cx))
    }

    #[gpui::test]
    fn a_rich_pane_s_editor_holds_its_text_but_neither_draws_nor_takes_the_keys(
        cx: &mut TestAppContext
    ) {
        let (panes, _heard, window) = rich_pane(cx);
        let pane = PaneId::new(PANE).unwrap();
        assert_eq!(
            text_of(&panes, window).as_deref(),
            Some(TITLE),
            "the page reads it"
        );
        let hidden = panes
            .panes
            .borrow()
            .get(&pane)
            .and_then(Followed::view)
            .cloned();
        let hidden = hidden.expect("the text has its editor");
        window.update(|window, cx| panes.focus(&pane, window, cx));
        let keys = window.update(|window, cx| hidden.focus_handle(cx).is_focused(window));
        assert!(!keys, "the page keeps the keyboard");
        let drawn = window.update(|window, cx| panes.content(&pane, window, cx).is_some());
        assert!(!drawn, "the page draws a rich pane");
    }

    #[gpui::test]
    fn the_rich_view_edits_the_one_text_that_both_views_show(cx: &mut TestAppContext) {
        let (panes, heard, window) = rich_pane(cx);
        let pane = PaneId::new(PANE).unwrap();
        window.update(|window, cx| panes.take_page_text((&pane, "# Edited\n"), window, cx));
        let dirty = ShellRequest::Document(DocumentRequest::Dirty {
            pane: pane.clone(),
            dirty: true,
        });
        assert_eq!(
            heard.try_recv().ok(),
            Some(dirty),
            "the edit dirties the pane"
        );
        assert!(panes.show(&pane, PaneView::Source), "to source");
        assert_eq!(kind(&panes, &pane), Some(PaneKind::Native));
        assert_eq!(
            text_of(&panes, window),
            None,
            "the native editor draws it now"
        );
        assert!(panes.show(&pane, PaneView::Rich), "back to rich");
        assert_eq!(
            text_of(&panes, window).as_deref(),
            Some("# Edited\n"),
            "one text"
        );
        panes.follow(&layout::Layout::default(), "");
        assert_eq!(kind(&panes, &pane), None, "closing forgets the view");
    }

    #[gpui::test]
    fn markdown_too_large_for_a_page_frame_opens_in_the_native_editor(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(editorview::init);
        let documents = Rc::new(FakeDocuments::default());
        documents.loading("c1");
        documents.holding(README, &"x".repeat(protocol::MAX_FRAME_BYTES));
        let (panes, heard) = panes_over(&documents);
        panes.follow(&layout_with(json!({"path": README})), "");
        landed(&panes, &heard);
        let pane = PaneId::new(PANE).unwrap();
        let (_root, window) = cx.add_window_view(|_, _| gpui::Empty);
        let flipped = window.update(|window, cx| panes.prepare(&pane, window, cx));
        assert!(flipped, "flipped to source");
        assert_eq!(kind(&panes, &pane), Some(PaneKind::Native));
    }

    #[test]
    fn the_rich_or_source_view_belongs_to_the_document_and_goes_with_it() {
        let documents = Rc::new(FakeDocuments::default());
        let (panes, _heard) = panes_over(&documents);
        let (one, two) = (PaneId::new(PANE).unwrap(), PaneId::new("f2").unwrap());
        panes.follow(&two_panes_on(README), "");
        assert!(panes.show(&one, PaneView::Source), "to source");
        assert_eq!(
            kind(&panes, &two),
            Some(PaneKind::Native),
            "both views flip"
        );
        assert!(
            panes.show(&two, PaneView::Rich),
            "back to rich from the other"
        );
        assert_eq!(kind(&panes, &one), Some(PaneKind::Web));
        assert!(panes.show(&two, PaneView::Source));
        panes.follow(&layout::Layout::default(), "");
        panes.follow(&layout_with(json!({"path": README})), "");
        assert_eq!(
            kind(&panes, &one),
            Some(PaneKind::Web),
            "a reopened file is rich"
        );
    }

    #[test]
    fn only_markdown_toggles_and_a_preview_stays_a_preview() {
        let cases = [
            (json!({"path": "src/a.ts"}), PaneView::Rich, false),
            (json!({"path": README}), PaneView::Rich, false),
            (
                json!({"path": README, "preview": "markdown"}),
                PaneView::Source,
                false,
            ),
            (json!({"path": README}), PaneView::Source, true),
        ];
        let pane = PaneId::new(PANE).unwrap();
        for (state, view, expected) in cases {
            let documents = Rc::new(FakeDocuments::default());
            let (panes, _heard) = panes_over(&documents);
            panes.follow(&layout_with(state.clone()), "");
            assert_eq!(panes.show(&pane, view), expected, "{state} to {view:?}");
        }
    }
}
