use gpui::{Context, Entity, Window};
use gpui_kit::base::input::Position;
use gpui_kit::component::input::EditorState;

use super::EditorView;
use crate::keys::GoToLine;

impl EditorView {
    pub(super) fn go_to_line(
        &mut self,
        _: &GoToLine,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.goto.update(cx, |goto, cx| goto.open(window, cx));
    }
}

pub(super) fn reveal_line(
    state: &Entity<EditorState>,
    line: u32,
    (window, cx): (&mut Window, &mut Context<'_, EditorView>),
) {
    let position = Position::new(line.saturating_sub(1), 0);
    state.update(cx, |state, cx| {
        state.set_cursor_position(position, window, cx);
    });
}

impl EditorView {
    /// Puts the caret at a language server's `(line, character)`, counted in UTF-16 code units
    /// as LSP counts them, as develop reveals a definition in a tab already open.
    pub fn reveal_lsp_position(
        &mut self,
        (line, character): (u32, u32),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let text = self.state.read(cx).text().clone();
        let position = crate::lsp_position::from_lsp(&text, Position::new(line, character));
        self.state.update(cx, |state, cx| {
            state.set_cursor_position(position, window, cx);
        });
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::Access;
    use crate::editor_view::test_support::opened;

    #[gpui::test]
    fn a_line_to_reveal_and_ctrl_g_put_the_caret_on_their_line(cx: &mut TestAppContext) {
        let cases = [("", 3), ("ctrl-g 2 enter", 2)];
        for (keys, line) in cases {
            let (view, window) = opened(("a\nb\nc\nd", Some(3)), Access::Editable, cx);
            window.run_until_parked();
            window.simulate_keystrokes(keys);
            assert_eq!(
                view.read_with(window, |view, cx| view.caret(cx).line),
                line,
                "{keys}"
            );
        }
    }

    #[gpui::test]
    fn a_language_server_position_past_an_emoji_puts_the_caret_on_its_character(
        cx: &mut TestAppContext
    ) {
        let (view, window) = opened(("a\n\"😀\" b", None), Access::Editable, cx);
        window.update(|window, cx| {
            view.update(cx, |view, cx| view.reveal_lsp_position((1, 5), window, cx));
        });
        assert_eq!(
            crate::editor_view::test_support::selected_of(&view, window),
            9..9
        );
    }
}
