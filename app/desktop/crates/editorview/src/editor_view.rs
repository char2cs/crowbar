use gpui::{
    App, AppContext as _, Context, Entity, EventEmitter, FocusHandle, Focusable,
    InteractiveElement as _, IntoElement, ParentElement as _, Render, Styled as _, Subscription,
    Window, div, px,
};
use gpui_kit::base::input::{InputEditsEvent, Rope};
use gpui_kit::component::Sizable as _;
use gpui_kit::component::input::{Editor, EditorState};

use crate::document::Document;
use crate::find_widget::FindWidget;
use crate::goto_line::GotoLine;
use crate::occurrence_session::OccurrenceSession;
use crate::save_key::{CONTEXT, Save};
use crate::theme::{ground, ink};
use crate::toolbar::Toolbar;
use crate::{Access, Caret, DocumentEvent, EditorEvent, Opening, Settings};

const BODY: &str = "editor-body";
const TEXT: &str = "editor-text";

mod access_change;
mod command_actions;
mod find_actions;
mod fold_command;
mod folding;
mod goto_action;
mod language;
mod line_command;
mod line_commands;
mod occurrences;
pub(crate) mod replay;
mod selections;
mod settings_follow;
#[cfg(test)]
pub(crate) mod test_support;
mod view_button;
mod view_buttons;

/// One pane's editor: the gpui-kit `Editor` drawn over a document, under the toolbar of its
/// file. Several views over one document share its text, each with its own carets and history.
pub struct EditorView {
    document: Entity<Document>,
    state: Entity<EditorState>,
    access: Access,
    toolbar: Toolbar,
    occurrences: Option<OccurrenceSession>,
    find: Entity<FindWidget>,
    goto: Entity<GotoLine>,
    _caret_moves: Subscription,
    _typed: Subscription,
    _replayed: Subscription,
    _document: Subscription,
    _settings: Subscription,
}

impl EditorView {
    /// A view over `document`, coloured by its language, with the caret on `opening`'s line.
    pub fn new(
        document: Entity<Document>,
        opening: Opening,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let Opening { line, access } = opening;
        let toolbar = Toolbar::of(document.read(cx).path());
        let language = toolbar.language();
        let shared = document.read(cx).shared_text().clone();
        let text = shared.read(cx).text().to_string();
        let state = cx.new(|cx| editor_state(language, text, (window, cx)));
        if let Some(line) = line {
            goto_action::reveal_line(&state, line, (window, cx));
        }
        let find = cx.new(|cx| FindWidget::new(state.clone(), window, cx));
        let goto = cx.new(|cx| GotoLine::new(state.clone(), window, cx));
        let caret_moves = cx.observe(&state, |_, _, cx| cx.notify());
        let typed = cx.subscribe(&state, |view, state, _: &InputEditsEvent, cx| {
            view.typed(&state, cx);
        });
        let replayed = cx.subscribe_in(&shared, window, |view, _, text: &Rope, window, cx| {
            view.replay(text, window, cx);
        });
        let followed = cx.subscribe(&document, |view, _, event, cx| {
            view.follow_document(*event, cx);
        });
        let settings = cx.observe_global_in::<Settings>(window, Self::follow_settings);
        let view = Self {
            document,
            state,
            access,
            toolbar,
            occurrences: None,
            find,
            goto,
            _caret_moves: caret_moves,
            _typed: typed,
            _replayed: replayed,
            _document: followed,
            _settings: settings,
        };
        view.ask_language_server(cx);
        view
    }

    /// The document the view shows.
    #[must_use]
    pub const fn document(&self) -> &Entity<Document> {
        &self.document
    }

    fn follow_document(
        &mut self,
        event: DocumentEvent,
        cx: &mut Context<'_, Self>,
    ) {
        match event {
            DocumentEvent::DirtyChanged(dirty) => cx.emit(EditorEvent::DirtyChanged(dirty)),
            DocumentEvent::EditsPaused => cx.emit(EditorEvent::EditsPaused),
            DocumentEvent::Rebound => self.rebound(cx),
        }
    }

    fn rebound(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let toolbar = Toolbar::of(self.document.read(cx).path());
        let language = crate::highlight_name::of(toolbar.language());
        self.state
            .update(cx, |state, cx| state.set_highlighter(language, cx));
        self.toolbar = toolbar;
        cx.notify();
    }

    fn typed(
        &self,
        state: &Entity<EditorState>,
        cx: &mut Context<'_, Self>,
    ) {
        let text = state.read(cx).text().clone();
        let shared = self.document.read(cx).shared_text().clone();
        shared.update(cx, |shared, cx| shared.set(text, cx));
    }

    fn replay(
        &self,
        text: &Rope,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let text = text.to_string();
        self.state.update(cx, |state, cx| {
            replay::replace_keeping_selections(state, &text, window, cx);
        });
    }

    /// The text the view holds now.
    #[must_use]
    pub fn text(
        &self,
        cx: &App,
    ) -> String {
        self.state.read(cx).value().to_string()
    }

    /// Where the caret is.
    #[must_use]
    pub fn caret(
        &self,
        cx: &App,
    ) -> Caret {
        let position = self.state.read(cx).cursor_position();
        Caret {
            line: position.line.saturating_add(1),
            column: position.character.saturating_add(1),
        }
    }
}

impl std::fmt::Debug for EditorView {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("EditorView")
            .field("access", &self.access)
            .finish_non_exhaustive()
    }
}

impl EventEmitter<EditorEvent> for EditorView {}

impl Focusable for EditorView {
    fn focus_handle(
        &self,
        cx: &App,
    ) -> FocusHandle {
        Focusable::focus_handle(self.state.read(cx), cx)
    }
}

impl Render for EditorView {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let read_only = self.access == Access::ReadOnly;
        let caret = self.caret(cx);
        let actions = self.toolbar_actions(cx);
        let rich_view = self.rich_view_button(cx);
        let toolbar = self.toolbar.element(caret, (actions, rich_view), cx);
        let settings = Settings::of(cx);
        let editor = Editor::new(&self.state)
            .appearance(false)
            .xsmall()
            .readonly(read_only)
            .size_full()
            .font_family(ui::Theme::of(cx).mono_family())
            .text_size(px(settings.font_size))
            .line_height(settings.row_height());
        let root = div()
            .size_full()
            .bg(ground(cx))
            .flex()
            .flex_col()
            .key_context(CONTEXT)
            .on_action(cx.listener(|_, _: &Save, _, cx| cx.emit(EditorEvent::SaveRequested)));
        let root = Self::command_actions(root, cx).on_action(cx.listener(Self::go_to_line));
        let text = div()
            .debug_selector(|| TEXT.to_owned())
            .flex_1()
            .min_h_0()
            .child(editor);
        Self::find_actions(root, cx).child(toolbar).child(
            div()
                .debug_selector(|| BODY.to_owned())
                .relative()
                .flex()
                .flex_col()
                .flex_1()
                .min_h_0()
                .text_color(ink(cx))
                .child(self.find.clone())
                .child(text)
                .child(self.goto.clone()),
        )
    }
}

fn editor_state(
    language: editor::Language,
    text: String,
    (window, cx): (&mut Window, &mut Context<'_, EditorState>),
) -> EditorState {
    let mut state = EditorState::new(window, cx)
        .language(crate::highlight_name::of(language))
        .context_menu(false)
        .searchable(false);
    state.set_value(text, window, cx);
    settings_follow::apply(&mut state, &Settings::of(cx), (window, cx));
    state
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::test_support::{
        document_of, document_opening, listened, opened, opened_at, press, shown,
    };
    use super::*;
    use crate::DocumentOpening;

    const MONACO_LINE_NUMBER_CHARS: f32 = 5.;
    const MONACO_FOLD_STRIP: gpui::Pixels = px(16.);

    fn language_of(
        view: &Entity<EditorView>,
        cx: &App,
    ) -> &'static str {
        view.read(cx).toolbar.language().display_name()
    }

    #[gpui::test]
    fn an_untitled_view_shows_its_name_as_plain_text(cx: &mut TestAppContext) {
        let (view, cx) = opened_at("untitled:Untitled-1", cx);
        assert_eq!(cx.read(|cx| language_of(&view, cx)), "Plain Text");
    }

    #[gpui::test]
    fn a_view_holds_the_text_it_was_given(cx: &mut TestAppContext) {
        let (view, cx) = opened(("a\nb", None), Access::ReadOnly, cx);
        cx.run_until_parked();
        assert_eq!(view.read_with(cx, EditorView::text), "a\nb");
    }

    #[gpui::test]
    fn typing_changes_an_editable_view_and_leaves_a_read_only_one(cx: &mut TestAppContext) {
        let cases = [(Access::Editable, "xyza"), (Access::ReadOnly, "a")];
        for (access, expected) in cases {
            let (view, window) = opened(("a", None), access, cx);
            window.update(|window, cx| window.focus(&view.focus_handle(cx), cx));
            window.simulate_input("xyz");
            assert_eq!(
                view.read_with(window, EditorView::text),
                expected,
                "{access:?}"
            );
        }
    }

    #[gpui::test]
    fn typing_marks_the_view_dirty_saving_cleans_it_and_ctrl_s_asks_to_save(
        cx: &mut TestAppContext
    ) {
        let (view, window) = opened(("a", None), Access::Editable, cx);
        let heard = listened(&view, window);
        let document = document_of(&view, window);
        window.simulate_input("x");
        assert!(document.read_with(window, Document::is_dirty), "typed");
        document.update(window, |document, cx| document.mark_saved("xa", cx));
        assert!(!document.read_with(window, Document::is_dirty), "saved");
        press("@editorview::Save", window);
        let expected = [
            EditorEvent::DirtyChanged(true),
            EditorEvent::DirtyChanged(false),
            EditorEvent::SaveRequested,
        ];
        assert_eq!(*heard.borrow(), expected);
    }

    #[gpui::test]
    fn a_view_opened_on_a_draft_is_dirty_and_undoes_nothing_back_to_its_file(
        cx: &mut TestAppContext
    ) {
        let draft = DocumentOpening {
            saved: Some("disk".to_owned()),
            ..document_opening("src/a.ts", "draft")
        };
        let opening = Opening {
            line: None,
            access: Access::Editable,
        };
        let (view, window) = shown((draft, opening), None, cx);
        let heard = listened(&view, window);
        let document = document_of(&view, window);
        assert!(
            document.read_with(window, Document::is_dirty),
            "opened dirty"
        );
        press("@input::Undo", window);
        assert_eq!(
            view.read_with(window, EditorView::text),
            "draft",
            "nothing to undo"
        );
        document.update(window, |document, cx| document.mark_saved("draft", cx));
        assert_eq!(
            document.read_with(window, |document, _| document.saved_text()),
            "draft"
        );
        assert_eq!(*heard.borrow(), [EditorEvent::DirtyChanged(false)]);
    }

    #[gpui::test]
    fn a_rebound_view_follows_its_new_file_s_language(cx: &mut TestAppContext) {
        let (view, window) = opened(("a", None), Access::Editable, cx);
        let language =
            |window: &mut gpui::VisualTestContext| window.read(|cx| language_of(&view, cx));
        assert_eq!(language(window), "TypeScript", "src/a.ts");
        let document = document_of(&view, window);
        document.update(window, |document, cx| document.rebind("notes.md", cx));
        assert_eq!(language(window), "Markdown");
    }

    #[gpui::test]
    fn the_editor_fills_its_pane_below_the_toolbar(cx: &mut TestAppContext) {
        let text = "line\n".repeat(200);
        let (view, window) = opened((&text, None), Access::Editable, cx);
        window.run_until_parked();
        let rows = window.read(|cx| view.read(cx).state.read(cx).visible_row_range());
        let shown = rows.map_or(0, |rows| rows.len());
        assert!(shown > 10, "only {shown} rows show");
    }

    #[gpui::test]
    fn the_text_starts_after_five_digit_cells_and_the_fold_strip_as_monaco_s_does(
        cx: &mut TestAppContext
    ) {
        let (view, window) = opened(("0\nb", None), Access::Editable, cx);
        window.run_until_parked();
        let first = window.read(|cx| view.read(cx).state.read(cx).range_to_bounds(&(0..1)));
        let (first, body) = (first.unwrap(), window.debug_bounds(BODY).unwrap());
        let digits = first.size.width * MONACO_LINE_NUMBER_CHARS;
        let offset = f32::from(first.left() - body.left());
        let expected = f32::from(digits + MONACO_FOLD_STRIP);
        assert!((offset - expected).abs() < 0.01, "{offset} != {expected}");
    }

    #[gpui::test]
    fn the_first_line_sits_at_the_top_of_the_editor_with_no_frame_as_develop_s_does(
        cx: &mut TestAppContext
    ) {
        let (view, window) = opened(("a\nb", None), Access::Editable, cx);
        window.run_until_parked();
        let first = window.read(|cx| view.read(cx).state.read(cx).range_to_bounds(&(0..1)));
        let body = window.debug_bounds(BODY);
        assert_eq!(first.map(|first| first.top()), body.map(|body| body.top()));
    }
}
