//! One open file's text and undo history, shown by every view over it, as develop's document
//! registry keeps one Monaco model per document.

use gpui::{App, AppContext as _, Context, Entity, Window};
use gpui_kit::base::input::TextModel;
use gpui_kit::component::input::EditorState;

pub(crate) struct Document {
    model: Entity<TextModel>,
}

impl Document {
    pub(crate) fn new(
        text: &str,
        cx: &mut App,
    ) -> Self {
        Self {
            model: cx.new(|_| TextModel::new(text)),
        }
    }

    pub(crate) fn show_in(
        &self,
        state: &mut EditorState,
        (window, cx): (&mut Window, &mut Context<'_, EditorState>),
    ) {
        state.share_text(self.model.clone(), window, cx);
    }
}

#[cfg(test)]
mod tests {
    use gpui::{
        IntoElement, ParentElement as _, Render, Styled as _, TestAppContext, VisualTestContext,
        div,
    };
    use gpui_kit::base::input::Position;
    use gpui_kit::component::input::Editor;

    use super::*;

    const TEXT: &str = "one\ntwo\n";

    struct Pair {
        a: Entity<EditorState>,
        b: Entity<EditorState>,
    }

    impl Render for Pair {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            div()
                .size_full()
                .child(Editor::new(&self.a))
                .child(Editor::new(&self.b))
        }
    }

    fn over(
        document: &Document,
        (window, cx): (&mut Window, &mut Context<'_, Pair>),
    ) -> Entity<EditorState> {
        cx.new(|cx| {
            let mut state = EditorState::new(window, cx);
            document.show_in(&mut state, (window, cx));
            state
        })
    }

    fn pair(cx: &mut TestAppContext) -> (Entity<Pair>, &mut VisualTestContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        cx.update(crate::init);
        cx.add_window_view(|window, cx| {
            let document = Document::new(TEXT, cx);
            let a = over(&document, (window, cx));
            let b = over(&document, (window, cx));
            Pair { a, b }
        })
    }

    fn states(
        pair: &Entity<Pair>,
        window: &mut VisualTestContext,
    ) -> (Entity<EditorState>, Entity<EditorState>) {
        window.read(|cx| (pair.read(cx).a.clone(), pair.read(cx).b.clone()))
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
    fn two_states_over_one_model_show_each_others_edits_and_keep_their_own_carets(
        cx: &mut TestAppContext
    ) {
        let (pair, window) = pair(cx);
        let (a, b) = states(&pair, window);
        put_caret(&b, (1, 2), window);
        put_caret(&a, (1, 0), window);
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
        window.simulate_input("x");
        put_caret(&b, (1, 0), window);
        window.simulate_keystrokes("ctrl-z");
        assert_eq!(text(&a, window), TEXT, "b undid a's typing");
        assert_eq!(text(&b, window), TEXT);
    }

    #[gpui::test]
    fn a_replacement_of_the_whole_text_keeps_every_states_caret(cx: &mut TestAppContext) {
        let (pair, window) = pair(cx);
        let (a, b) = states(&pair, window);
        put_caret(&b, (1, 1), window);
        a.update_in(window, |state, window, cx| {
            state.replace_all_keeping_selections("one\ntwo\nthree\n", window, cx);
        });
        assert_eq!(text(&b, window), "one\ntwo\nthree\n");
        assert_eq!(
            caret(&b, window),
            Position {
                line: 1,
                character: 1
            }
        );
    }
}
