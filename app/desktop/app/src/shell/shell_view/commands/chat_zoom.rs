use gpui::{Context, KeyDownEvent, Window};

use super::shell_commands::heard;
use crate::shell::shell_view::ShellView;

impl ShellView {
    pub(crate) fn on_key_alongside(
        &mut self,
        event: &KeyDownEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if event.is_held {
            return;
        }
        let stroke = heard(&event.keystroke, cx);
        if let Some(command) = self.keymap.alongside(&stroke) {
            self.run_shell_command(command, window, cx);
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use keymap::ShellCommand;

    use crate::shell::shell_view::test_support::{keys_of, with_terminal};

    #[gpui::test]
    fn the_chat_zoom_chords_reach_the_chats_from_a_focused_terminal(cx: &mut TestAppContext) {
        let (zoom_in, reset) = (
            keys_of(ShellCommand::ZoomChatIn),
            keys_of(ShellCommand::ZoomChatReset),
        );
        let cases = [
            (zoom_in.clone(), 110),
            (format!("{zoom_in} {zoom_in}"), 130),
            (reset, 100),
        ];
        let (mut opened, _) = with_terminal(cx);
        for (keys, percent) in cases {
            opened.window.simulate_keystrokes(&keys);
            opened.window.run_until_parked();
            let zoom = opened
                .shell
                .read_with(&opened.window, |shell, _| shell.state.host().chat_zoom());
            assert_eq!(zoom, percent, "{keys}");
        }
    }
}
