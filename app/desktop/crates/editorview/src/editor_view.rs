use gpui::{
    App, AppContext as _, Context, Entity, EventEmitter, FocusHandle, Focusable,
    InteractiveElement as _, IntoElement, ParentElement as _, Render, Styled as _, Subscription,
    Task, Window, div,
};
use gpui_kit::base::input::{InputEvent, Rope};
use gpui_kit::component::input::{Editor, EditorState};

use crate::find_widget::FindWidget;
use crate::goto_line::GotoLine;
use crate::occurrence_session::OccurrenceSession;
use crate::save_key::{CONTEXT, Save};
use crate::theme::ink;
use crate::toolbar::Toolbar;
use crate::{Access, Caret, EditorEvent, Opening, SaveMark};

mod access_change;
mod command_actions;
mod disk_text;
mod find_actions;
mod fold_command;
mod folding;
mod goto_action;
mod line_command;
mod line_commands;
mod occurrences;
mod page_text;
mod quiet;
#[cfg(test)]
pub(crate) mod test_support;
mod view_buttons;

/// One pane's editor: the gpui-kit `Editor` drawn over the document text it holds, under the
/// toolbar of its file.
pub struct EditorView {
    state: Entity<EditorState>,
    access: Access,
    toolbar: Toolbar,
    saved: Rope,
    save_mark: SaveMark,
    disk_conflict: bool,
    reported_dirty: bool,
    quiet: Option<Task<()>>,
    occurrences: Option<OccurrenceSession>,
    find: Entity<FindWidget>,
    goto: Entity<GotoLine>,
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
            saved,
        } = opening;
        let toolbar = Toolbar::of(&path);
        let language = toolbar.language();
        let saved = Rope::from(saved.as_deref().unwrap_or(&text));
        let reported_dirty = saved != text.as_str();
        let state = cx.new(|cx| editor_state(language, text, (window, cx)));
        if let Some(line) = line {
            goto_action::reveal_line(&state, line, (window, cx));
        }
        let find = cx.new(|cx| FindWidget::new(state.clone(), window, cx));
        let goto = cx.new(|cx| GotoLine::new(state.clone(), window, cx));
        let caret_moves = cx.observe(&state, |_, _, cx| cx.notify());
        let edits = cx.subscribe(&state, Self::on_input);
        Self {
            state,
            access,
            toolbar,
            saved,
            save_mark: SaveMark::default(),
            disk_conflict: false,
            reported_dirty,
            quiet: None,
            occurrences: None,
            find,
            goto,
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
        self.toolbar = toolbar;
        cx.notify();
    }

    /// `text` is now on disk: the view is clean again while it still holds that text.
    pub fn mark_saved(
        &mut self,
        text: &str,
        cx: &mut Context<'_, Self>,
    ) {
        self.saved = Rope::from(text);
        self.save_mark = disk_text::next_mark(self.save_mark);
        self.report_dirty(cx);
        if !self.reported_dirty {
            self.disk_conflict = false;
        }
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
        let actions = self.toolbar_actions(cx);
        let rich_view = self.rich_view_button(cx);
        let toolbar = self.toolbar.element(caret, (actions, rich_view), cx);
        let root = div()
            .size_full()
            .flex()
            .flex_col()
            .key_context(CONTEXT)
            .on_action(cx.listener(|_, _: &Save, _, cx| cx.emit(EditorEvent::SaveRequested)));
        let root = Self::command_actions(root, cx).on_action(cx.listener(Self::go_to_line));
        Self::find_actions(root, cx).child(toolbar).child(
            div()
                .relative()
                .flex_1()
                .min_h_0()
                .text_color(ink(cx))
                .child(Editor::new(&self.state).readonly(read_only).size_full())
                .child(self.find.clone())
                .child(self.goto.clone()),
        )
    }
}

fn editor_state(
    language: editor::Language,
    text: String,
    (window, cx): (&mut Window, &mut Context<'_, EditorState>),
) -> EditorState {
    EditorState::new(window, cx)
        .language(crate::highlight_name::of(language))
        .soft_wrap(false)
        .context_menu(false)
        .searchable(false)
        .default_value(text)
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::test_support::{listened, opened};
    use super::*;

    fn language_of(
        view: &Entity<EditorView>,
        cx: &App,
    ) -> &'static str {
        view.read(cx).toolbar.language().display_name()
    }

    #[gpui::test]
    fn an_untitled_view_shows_its_name_as_plain_text(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(crate::init);
        let opening = Opening {
            text: String::new(),
            path: "untitled:Untitled-1".to_owned(),
            line: None,
            access: Access::Editable,
            saved: None,
        };
        let (view, cx) = cx.add_window_view(|window, cx| EditorView::new(opening, window, cx));
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
    fn a_view_opened_on_a_draft_is_dirty_and_undoes_nothing_back_to_its_file(
        cx: &mut TestAppContext
    ) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(crate::init);
        let opening = Opening {
            text: "draft".to_owned(),
            path: "src/a.ts".to_owned(),
            line: None,
            access: Access::Editable,
            saved: Some("disk".to_owned()),
        };
        let (view, window) = cx.add_window_view(|window, cx| EditorView::new(opening, window, cx));
        let heard = listened(&view, window);
        assert!(view.read_with(window, EditorView::is_dirty), "opened dirty");
        window.simulate_keystrokes("ctrl-z");
        assert_eq!(
            view.read_with(window, EditorView::text),
            "draft",
            "nothing to undo"
        );
        window.update(|_, cx| view.update(cx, |view, cx| view.mark_saved("draft", cx)));
        assert_eq!(view.read_with(window, |view, _| view.saved_text()), "draft");
        assert_eq!(*heard.borrow(), [EditorEvent::DirtyChanged(false)]);
    }

    #[gpui::test]
    fn a_rebound_view_follows_its_new_file_s_language(cx: &mut TestAppContext) {
        let (view, window) = opened(("a", None), Access::Editable, cx);
        let language =
            |window: &mut gpui::VisualTestContext| window.read(|cx| language_of(&view, cx));
        assert_eq!(language(window), "TypeScript", "src/a.ts");
        window.update(|_, cx| view.update(cx, |view, cx| view.rebind("notes.md", cx)));
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
}
