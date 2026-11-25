//! One open file's text and undo history, shown by every view over it, and what is known of it:
//! the text last saved, the latest save, a change on disk, and its language server, as develop's
//! document registry keeps one Monaco model and its saved state per document.

use std::rc::Rc;

use gpui::{App, AppContext as _, Context, Entity, EventEmitter, Subscription, Task};
use gpui_kit::base::input::Rope;

use crate::lsp_link::LspLink;
use crate::{DocumentEvent, DocumentOpening, LanguageServer, SaveMark};

mod disk_text;
mod quiet;
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
        let lsp = server.map(|server| LspLink::new(server, shared.downgrade()));
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
        IntoElement, ParentElement as _, Render, Styled as _, TestAppContext, VisualTestContext,
        Window, div,
    };
    use gpui_kit::base::input::Position;
    use gpui::Focusable as _;
    use gpui_kit::component::input::EditorState;

    use super::*;
    use crate::editor_view::test_support::{listened, press, state_of};

    const TEXT: &str = "one\ntwo\n";

    fn opening(text: &str) -> DocumentOpening {
        DocumentOpening {
            text: text.to_owned(),
            path: "src/a.ts".to_owned(),
            saved: None,
        }
    }

    fn pair(cx: &mut TestAppContext) -> (Entity<Views>, &mut VisualTestContext) {
        two_views(None, cx)
    }

    fn states(
        views: &Entity<Views>,
        window: &mut VisualTestContext,
    ) -> (Entity<EditorState>, Entity<EditorState>) {
        window.read(|cx| {
            let views = views.read(cx);
            (state_of(&views.a, cx), state_of(&views.b, cx))
        })
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
    ) -> Position {
        window.read(|cx| state.read(cx).cursor_position())
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
        let (pair, window) = pair(cx);
        let (a, b) = states(&pair, window);
        put_caret(&b, (1, 2), window);
        put_caret(&a, (1, 0), window);
        focus(&a, window);
        window.simulate_input("xy");
        assert_eq!(text(&b, window), "one\nxytwo\n", "b shows a's typing");
        assert_eq!(
            caret(&b, window),
            Position {
                line: 1,
                character: 4
            },
            "b's caret moves on past a's typing"
        );
        assert_eq!(
            caret(&a, window),
            Position {
                line: 1,
                character: 2
            },
            "a's caret follows its own typing"
        );
    }

    #[gpui::test]
    fn undo_in_either_state_undoes_the_documents_last_edit(cx: &mut TestAppContext) {
        let (pair, window) = pair(cx);
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
        let (pair, window) = pair(cx);
        let (_, b) = states(&pair, window);
        put_caret(&b, (1, 1), window);
        let document = window.read(|cx| pair.read(cx).a.read(cx).document().clone());
        document.update(window, |document, cx| {
            document.take_page_text("one\ntwo\nthree\n", cx);
        });
        window.run_until_parked();
        assert_eq!(text(&b, window), "one\ntwo\nthree\n");
        assert_eq!(
            caret(&b, window),
            Position {
                line: 1,
                character: 1
            }
        );
    }

    struct Views {
        a: Entity<crate::EditorView>,
        b: Entity<crate::EditorView>,
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
    ) -> (Entity<Views>, &mut VisualTestContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(crate::init);
        let shown = crate::Opening {
            line: None,
            access: crate::Access::Editable,
        };
        cx.add_window_view(|window, cx| {
            let document = cx.new(|cx| Document::new(opening(TEXT), server, cx));
            let a = cx.new(|cx| crate::EditorView::new(document.clone(), shown, window, cx));
            let b = cx.new(|cx| crate::EditorView::new(document, shown, window, cx));
            Views { a, b }
        })
    }

    #[gpui::test]
    fn marking_the_document_saved_cleans_every_view_over_it(cx: &mut TestAppContext) {
        let (views, window) = two_views(None, cx);
        let (a, b) = window.read(|cx| (views.read(cx).a.clone(), views.read(cx).b.clone()));
        let (heard_a, heard_b) = (listened(&a, window), listened(&b, window));
        window
            .update(|window, cx| window.focus(&gpui::Focusable::focus_handle(a.read(cx), cx), cx));
        window.simulate_input("x");
        let document = window.read(|cx| b.read(cx).document().clone());
        document.update(window, |document, cx| {
            document.mark_saved("xone\ntwo\n", cx);
        });
        let expected = [
            crate::EditorEvent::DirtyChanged(true),
            crate::EditorEvent::DirtyChanged(false),
        ];
        assert_eq!(*heard_a.borrow(), expected, "a");
        assert_eq!(*heard_b.borrow(), expected, "b");
    }

    #[gpui::test]
    fn a_document_shown_twice_sends_one_did_open(cx: &mut TestAppContext) {
        let server = crate::editor_view::test_support::FakeServer::default();
        let (views, window) = two_views(Some(server.port()), cx);
        let a = window.read(|cx| views.read(cx).a.clone());
        window
            .update(|window, cx| window.focus(&gpui::Focusable::focus_handle(a.read(cx), cx), cx));
        window.simulate_input("x");
        window
            .executor()
            .advance_clock(std::time::Duration::from_secs(1));
        window.run_until_parked();
        let told = server.verbs();
        let document_verbs: Vec<&str> = told
            .into_iter()
            .filter(|verb| verb.starts_with("did"))
            .collect();
        assert_eq!(document_verbs, ["didOpen", "didChange"]);
    }
}
