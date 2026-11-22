use gpui::{
    App, AppContext as _, Context, Entity, EventEmitter, FocusHandle, Focusable,
    InteractiveElement as _, IntoElement, ParentElement as _, Render, Styled as _, Subscription,
    Task, Window, div,
};
use gpui_kit::base::input::{InputEvent, Rope};
use gpui_kit::component::input::{Editor, EditorState, Position};

use crate::save_key::{CONTEXT, Save};
use crate::toolbar::Toolbar;
use crate::{Access, Caret, EditorEvent, Opening};

mod quiet;

/// One pane's editor: the gpui-kit `Editor` drawn over the document text it holds, under the
/// toolbar of its file.
pub struct EditorView {
    state: Entity<EditorState>,
    access: Access,
    toolbar: Option<Toolbar>,
    saved: Rope,
    reported_dirty: bool,
    quiet: Option<Task<()>>,
    _caret_moves: Subscription,
    _edits: Subscription,
}

impl EditorView {
    /// A view showing `opening`'s text, coloured by its language, with the caret on its line.
    pub fn new(
        opening: Opening,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let Opening {
            text,
            path,
            line,
            access,
        } = opening;
        let toolbar = path.as_deref().map(Toolbar::of);
        let language = toolbar
            .as_ref()
            .map_or(editor::Language::of_path(""), Toolbar::language);
        let saved = Rope::from(text.as_str());
        let state = cx.new(|cx| {
            EditorState::new(window, cx)
                .language(crate::highlight_name::of(language))
                .soft_wrap(false)
                .context_menu(false)
                .default_value(text)
        });
        if let Some(line) = line {
            reveal(&state, line, (window, cx));
        }
        let caret_moves = cx.observe(&state, |_, _, cx| cx.notify());
        let edits = cx.subscribe(&state, Self::on_input);
        Self {
            state,
            access,
            toolbar,
            saved,
            reported_dirty: false,
            quiet: None,
            _caret_moves: caret_moves,
            _edits: edits,
        }
    }

    /// Whether the text differs from the text last saved (or opened).
    #[must_use]
    pub fn is_dirty(
        &self,
        cx: &App,
    ) -> bool {
        self.state.read(cx).text() != &self.saved
    }

    /// The view now shows the file at `path`: its toolbar and its language follow it.
    pub fn rebind(
        &mut self,
        path: &str,
        cx: &mut Context<'_, Self>,
    ) {
        let toolbar = Toolbar::of(path);
        let language = crate::highlight_name::of(toolbar.language());
        self.state
            .update(cx, |state, cx| state.set_highlighter(language, cx));
        self.toolbar = Some(toolbar);
        cx.notify();
    }

    /// `text` is now on disk: the view is clean again while it still holds that text.
    pub fn mark_saved(
        &mut self,
        text: &str,
        cx: &mut Context<'_, Self>,
    ) {
        self.saved = Rope::from(text);
        self.report_dirty(cx);
    }

    fn on_input(
        &mut self,
        _: Entity<EditorState>,
        event: &InputEvent,
        cx: &mut Context<'_, Self>,
    ) {
        if !matches!(event, InputEvent::Change) {
            return;
        }
        self.report_dirty(cx);
        self.quiet = Some(cx.spawn(async move |view, cx| Self::wait_quiet(view, cx).await));
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
        cx.emit(EditorEvent::DirtyChanged(dirty));
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
        let toolbar = self
            .toolbar
            .as_ref()
            .map(|toolbar| toolbar.element(caret, cx));
        div()
            .size_full()
            .flex()
            .flex_col()
            .key_context(CONTEXT)
            .on_action(cx.listener(|_, _: &Save, _, cx| cx.emit(EditorEvent::SaveRequested)))
            .children(toolbar)
            .child(
                div()
                    .flex_1()
                    .min_h_0()
                    .child(Editor::new(&self.state).readonly(read_only).size_full()),
            )
    }
}

fn reveal(
    state: &Entity<EditorState>,
    line: u32,
    (window, cx): (&mut Window, &mut Context<'_, EditorView>),
) {
    let position = Position::new(line.saturating_sub(1), 0);
    state.update(cx, |state, cx| {
        state.set_cursor_position(position, window, cx);
    });
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;

    pub(super) fn opened<'cx>(
        (text, line): (&str, Option<u32>),
        access: Access,
        cx: &'cx mut TestAppContext,
    ) -> (Entity<EditorView>, &'cx mut gpui::VisualTestContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(crate::init);
        let opening = Opening {
            text: text.to_owned(),
            path: Some("src/a.ts".to_owned()),
            line,
            access,
        };
        cx.add_window_view(|window, cx| EditorView::new(opening, window, cx))
    }

    fn language_of(
        view: &Entity<EditorView>,
        cx: &App,
    ) -> Option<&'static str> {
        let toolbar = view.read(cx).toolbar.as_ref();
        toolbar.map(|toolbar| toolbar.language().display_name())
    }

    pub(super) fn listened(
        view: &Entity<EditorView>,
        window: &mut gpui::VisualTestContext,
    ) -> std::rc::Rc<std::cell::RefCell<Vec<EditorEvent>>> {
        let heard = std::rc::Rc::new(std::cell::RefCell::new(Vec::new()));
        let log = std::rc::Rc::clone(&heard);
        let record = move |_: Entity<EditorView>, event: &EditorEvent, _: &mut App| {
            log.borrow_mut().push(*event);
        };
        window.update(|window, cx| {
            window.focus(&view.focus_handle(cx), cx);
            cx.subscribe(view, record).detach();
        });
        heard
    }

    #[gpui::test]
    fn a_view_holds_the_text_it_was_given(cx: &mut TestAppContext) {
        let (view, cx) = opened(("a\nb", None), Access::ReadOnly, cx);
        cx.run_until_parked();
        assert_eq!(view.read_with(cx, EditorView::text), "a\nb");
    }

    #[gpui::test]
    fn a_line_to_reveal_puts_the_caret_on_it(cx: &mut TestAppContext) {
        let (view, cx) = opened(("a\nb\nc\nd", Some(3)), Access::ReadOnly, cx);
        cx.run_until_parked();
        assert_eq!(view.read_with(cx, |view, cx| view.caret(cx).line), 3);
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
        window.simulate_input("x");
        assert!(view.read_with(window, EditorView::is_dirty), "typed");
        window.update(|_, cx| view.update(cx, |view, cx| view.mark_saved("xa", cx)));
        assert!(!view.read_with(window, EditorView::is_dirty), "saved");
        window.simulate_keystrokes("secondary-s");
        let expected = [
            EditorEvent::DirtyChanged(true),
            EditorEvent::DirtyChanged(false),
            EditorEvent::SaveRequested,
        ];
        assert_eq!(*heard.borrow(), expected);
    }

    #[gpui::test]
    fn a_rebound_view_follows_its_new_file_s_language(cx: &mut TestAppContext) {
        let (view, window) = opened(("a", None), Access::Editable, cx);
        let language =
            |window: &mut gpui::VisualTestContext| window.read(|cx| language_of(&view, cx));
        assert_eq!(language(window), Some("TypeScript"), "src/a.ts");
        window.update(|_, cx| view.update(cx, |view, cx| view.rebind("notes.md", cx)));
        assert_eq!(language(window), Some("Markdown"));
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
}
