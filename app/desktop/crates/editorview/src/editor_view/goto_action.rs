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
}
