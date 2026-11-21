use gpui::{
    App, AppContext as _, Context, Entity, FocusHandle, Focusable, IntoElement, ParentElement as _,
    Render, Styled as _, Subscription, Window, div,
};
use gpui_kit::component::input::{Editor, EditorState, Position};

use crate::toolbar::Toolbar;
use crate::{Access, Caret, Opening};

/// One pane's editor: the gpui-kit `Editor` drawn over the document text it holds, under the
/// toolbar of its file.
pub struct EditorView {
    state: Entity<EditorState>,
    access: Access,
    toolbar: Option<Toolbar>,
    _caret_moves: Subscription,
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
        Self {
            state,
            access,
            toolbar,
            _caret_moves: caret_moves,
        }
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
        div().size_full().flex().flex_col().children(toolbar).child(
            div()
                .flex_1()
                .min_h_0()
                .child(Editor::new(&self.state).readonly(read_only)),
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

    fn opened<'cx>(
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
}
