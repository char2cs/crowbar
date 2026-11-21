use gpui::{Context, KeyDownEvent, Window};

use super::ShellView;
use super::shell_commands::keystroke_of;

impl ShellView {
    pub(super) fn on_key_alongside(
        &mut self,
        event: &KeyDownEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if event.is_held {
            return;
        }
        let stroke = keystroke_of(&event.keystroke);
        if let Some(command) = self.keymap.alongside(&stroke) {
            self.run_shell_command(command, window, cx);
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::shell_view::test_support::with_terminal;

    #[gpui::test]
    fn the_chat_zoom_chords_reach_the_chats_from_a_focused_terminal(cx: &mut TestAppContext) {
        let cases: [(&str, u16); 3] = [("ctrl-=", 110), ("ctrl-= ctrl-=", 130), ("ctrl-0", 100)];
        let (mut opened, _) = with_terminal(cx);
        for (keys, percent) in cases {
            opened.window.simulate_keystrokes(keys);
            opened.window.run_until_parked();
            let zoom = opened
                .shell
                .read_with(&opened.window, |shell, _| shell.state.host().chat_zoom());
            assert_eq!(zoom, percent, "{keys}");
        }
    }
}
