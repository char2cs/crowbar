//! One open file's text and undo history, shown by every view over it, and what is known of it:
//! the text last saved, the latest save, a change on disk, and its language server, as develop's
//! document registry keeps one Monaco model and its saved state per document.

use std::rc::Rc;

use gpui::{App, AppContext as _, Context, Entity, EventEmitter, Subscription, Task};
use gpui_kit::base::input::Rope;

use crate::lsp::lsp_link::LspLink;
use crate::{DocumentOpening, LanguageServer, SaveMark};
use document_event::DocumentEvent;

pub(crate) mod disk_take;
mod disk_text;
pub(crate) mod document_event;
pub(crate) mod document_opening;
mod quiet;
pub(crate) mod save_mark;
mod shared_text;

pub(crate) use shared_text::SharedText;

/// One open document, shared by every view that shows it.
pub struct Document {
    text: Entity<SharedText>,
    path: String,
    saved: Rope,
    save_mark: SaveMark,
    disk_conflict: bool,
    reported_dirty: bool,
    quiet: Option<Task<()>>,
    lsp: Option<Rc<LspLink>>,
    _edits: Subscription,
}

impl Document {
    /// The document `opening` describes, told to `server` (when it has one) from now until it
    /// goes, as develop's `document-lsp.ts` attaches once per model.
    pub fn new(
        opening: DocumentOpening,
        server: Option<Rc<dyn LanguageServer>>,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let DocumentOpening { text, path, saved } = opening;
        let saved = Rope::from(saved.as_deref().unwrap_or(&text));
        let reported_dirty = saved != text.as_str();
        let shared = cx.new(|_| SharedText::new(&text));
        let edits = cx.subscribe(&shared, Self::on_edit);
        let reader = shared.downgrade();
        let read = move |cx: &App| Some(reader.upgrade()?.read(cx).text().to_string());
        let lsp = server.map(|server| LspLink::new(server, Box::new(read)));
        if let Some(link) = &lsp {
            link.open(&path, cx);
        }
        cx.on_release(|document, cx| document.lsp_close(cx))
            .detach();
        Self {
            text: shared,
            path,
            saved,
            save_mark: SaveMark::default(),
            disk_conflict: false,
            reported_dirty,
            quiet: None,
            lsp,
            _edits: edits,
        }
    }

    pub(crate) const fn shared_text(&self) -> &Entity<SharedText> {
        &self.text
    }

    pub(crate) fn lsp(&self) -> Option<Rc<LspLink>> {
        self.lsp.clone()
    }

    /// The path the document is the file at, or its `untitled:` name.
    #[must_use]
    pub fn path(&self) -> &str {
        &self.path
    }

    /// The text the document holds now.
    #[must_use]
    pub fn text(
        &self,
        cx: &App,
    ) -> String {
        self.text.read(cx).text().to_string()
    }

    /// Whether the text differs from the text last saved (or opened).
    #[must_use]
    pub fn is_dirty(
        &self,
        cx: &App,
    ) -> bool {
        self.text.read(cx).text() != &self.saved
    }

    /// The document is now the file at `path`: its language and its language server follow it.
    pub fn rebind(
        &mut self,
        path: &str,
        cx: &mut Context<'_, Self>,
    ) {
        path.clone_into(&mut self.path);
        if let Some(link) = &self.lsp {
            link.open(path, cx);
        }
        cx.emit(DocumentEvent::Rebound);
    }

    /// `text` is now on disk: the document is clean again while it still holds that text.
    pub fn mark_saved(
        &mut self,
        text: &str,
        cx: &mut Context<'_, Self>,
    ) {
        self.saved = Rope::from(text);
        self.save_mark = disk_text::next_mark(self.save_mark);
        if let Some(link) = &self.lsp {
            link.saved(cx);
        }
        self.report_dirty(cx);
        if !self.reported_dirty {
            self.disk_conflict = false;
        }
    }

    fn on_edit(
        &mut self,
        _: Entity<SharedText>,
        _: &Rope,
        cx: &mut Context<'_, Self>,
    ) {
        self.report_dirty(cx);
        if let Some(link) = &self.lsp {
            link.changed(cx);
        }
        self.quiet = Some(cx.spawn(async move |document, cx| Self::wait_quiet(document, cx).await));
    }

    fn report_dirty(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let dirty = self.is_dirty(cx);
        if dirty == self.reported_dirty {
            return;
        }
        self.reported_dirty = dirty;
        cx.emit(DocumentEvent::DirtyChanged(dirty));
    }

    fn lsp_close(
        &self,
        cx: &mut App,
    ) {
        if let Some(link) = &self.lsp {
            link.close(cx);
        }
    }
}

impl EventEmitter<DocumentEvent> for Document {}

impl std::fmt::Debug for Document {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("Document")
            .field("path", &self.path)
            .field("save_mark", &self.save_mark)
            .finish_non_exhaustive()
    }
}

#[cfg(test)]
mod tests {
    use gpui::{
        Focusable as _, IntoElement, ParentElement as _, Render, Styled as _, TestAppContext,
        VisualTestContext, Window, div,
    };
    use gpui_kit::base::input::Position;
    use gpui_kit::component::input::EditorState;

    use super::*;
    use crate::editor_view::test_support::{FakeServer, listened, press, state_of};
    use crate::{Access, EditorEvent, EditorView, Opening};

    const TEXT: &str = "one\ntwo\n";

    type Pair = (Entity<EditorView>, Entity<EditorView>);

    struct Views {
        a: Entity<EditorView>,
        b: Entity<EditorView>,
    }

    impl Render for Views {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            div()
                .size_full()
                .child(self.a.clone())
                .child(self.b.clone())
        }
    }

    fn two_views(
        server: Option<Rc<dyn LanguageServer>>,
        cx: &mut TestAppContext,
    ) -> (Pair, &mut VisualTestContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(crate::init);
        let shown = Opening {
            line: None,
            access: Access::Editable,
        };
        let opening = DocumentOpening {
            text: TEXT.to_owned(),
            path: "src/a.ts".to_owned(),
            saved: None,
        };
        let (views, window) = cx.add_window_view(|window, cx| {
            let document = cx.new(|cx| Document::new(opening, server, cx));
            let a = cx.new(|cx| EditorView::new(document.clone(), shown, window, cx));
            let b = cx.new(|cx| EditorView::new(document, shown, window, cx));
            Views { a, b }
        });
        let pair = window.read(|cx| (views.read(cx).a.clone(), views.read(cx).b.clone()));
        (pair, window)
    }

    fn states(
        (a, b): &Pair,
        window: &mut VisualTestContext,
    ) -> (Entity<EditorState>, Entity<EditorState>) {
        window.read(|cx| (state_of(a, cx), state_of(b, cx)))
    }

    fn focus(
        state: &Entity<EditorState>,
        window: &mut VisualTestContext,
    ) {
        window.update(|window, cx| window.focus(&state.read(cx).focus_handle(cx), cx));
    }

    fn put_caret(
        state: &Entity<EditorState>,
        (line, character): (u32, u32),
        window: &mut VisualTestContext,
    ) {
        state.update_in(window, |state, window, cx| {
            state.set_cursor_position(Position { line, character }, window, cx);
        });
    }

    fn caret(
        state: &Entity<EditorState>,
        window: &mut VisualTestContext,
    ) -> (u32, u32) {
        let at = window.read(|cx| state.read(cx).cursor_position());
        (at.line, at.character)
    }

    fn text(
        state: &Entity<EditorState>,
        window: &mut VisualTestContext,
    ) -> String {
        window.read(|cx| state.read(cx).value().to_string())
    }

    #[gpui::test]
    fn two_views_over_one_document_show_each_others_edits_and_keep_their_own_carets(
        cx: &mut TestAppContext
    ) {
        let (pair, window) = two_views(None, cx);
        let (a, b) = states(&pair, window);
        put_caret(&b, (1, 2), window);
        put_caret(&a, (1, 0), window);
        focus(&a, window);
        window.simulate_input("xy");
        assert_eq!(text(&b, window), "one\nxytwo\n", "b shows a's typing");
        assert_eq!(
            caret(&b, window),
            (1, 4),
            "b's caret moves on past a's typing"
        );
        assert_eq!(
            caret(&a, window),
            (1, 2),
            "a's caret follows its own typing"
        );
    }

    #[gpui::test]
    fn undo_in_either_state_undoes_the_documents_last_edit(cx: &mut TestAppContext) {
        let (pair, window) = two_views(None, cx);
        let (a, b) = states(&pair, window);
        put_caret(&a, (0, 0), window);
        focus(&a, window);
        window.simulate_input("x");
        put_caret(&b, (1, 0), window);
        focus(&b, window);
        press("@input::Undo", window);
        assert_eq!(text(&a, window), TEXT, "b undid a's typing");
        assert_eq!(text(&b, window), TEXT);
    }

    #[gpui::test]
    fn a_replacement_of_the_whole_text_keeps_every_states_caret(cx: &mut TestAppContext) {
        let (pair, window) = two_views(None, cx);
        let (_, b) = states(&pair, window);
        put_caret(&b, (1, 1), window);
        let document = window.read(|cx| pair.0.read(cx).document().clone());
        document.update(window, |document, cx| {
            document.take_page_text("one\ntwo\nthree\n", cx);
        });
        window.run_until_parked();
        assert_eq!(text(&b, window), "one\ntwo\nthree\n");
        assert_eq!(caret(&b, window), (1, 1));
    }

    #[gpui::test]
    fn marking_the_document_saved_cleans_every_view_over_it(cx: &mut TestAppContext) {
        let ((a, b), window) = two_views(None, cx);
        let (heard_a, heard_b) = (listened(&a, window), listened(&b, window));
        window.update(|window, cx| window.focus(&a.focus_handle(cx), cx));
        window.simulate_input("x");
        let document = window.read(|cx| b.read(cx).document().clone());
        document.update(window, |document, cx| {
            document.mark_saved("xone\ntwo\n", cx);
        });
        let expected = [
            EditorEvent::DirtyChanged(true),
            EditorEvent::DirtyChanged(false),
        ];
        assert_eq!(*heard_a.borrow(), expected, "a");
        assert_eq!(*heard_b.borrow(), expected, "b");
    }

    #[gpui::test]
    fn a_document_shown_twice_sends_one_did_open(cx: &mut TestAppContext) {
        let server = FakeServer::default();
        let ((a, _), window) = two_views(Some(server.port()), cx);
        window.update(|window, cx| window.focus(&a.focus_handle(cx), cx));
        window.simulate_input("x");
        window
            .executor()
            .advance_clock(std::time::Duration::from_secs(1));
        window.run_until_parked();
        let verbs = server.verbs();
        let document_verbs: Vec<&str> = verbs
            .into_iter()
            .filter(|verb| verb.starts_with("did"))
            .collect();
        assert_eq!(document_verbs, ["didOpen", "didChange"]);
    }
}
