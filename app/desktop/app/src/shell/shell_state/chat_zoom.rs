use keymap::ShellCommand;

use super::ShellState;

const STEP: u16 = 10;
const LEAST: u16 = 30;
const MOST: u16 = 300;
const UNZOOMED: u16 = 100;

impl ShellState {
    pub(crate) fn zoom_chat(
        &mut self,
        command: ShellCommand,
    ) {
        let now = self.host.chat_zoom();
        let next = match command {
            ShellCommand::ZoomChatIn => now.saturating_add(STEP).min(MOST),
            ShellCommand::ZoomChatOut => now.saturating_sub(STEP).max(LEAST),
            ShellCommand::ZoomChatReset => UNZOOMED,
            _ => return,
        };
        if let Err(error) = self.host.set_chat_zoom(next) {
            tracing::warn!(%error, "sending the chat zoom to panes failed");
        }
    }
}

#[cfg(test)]
mod tests {
    use keymap::ShellCommand::{ZoomChatIn, ZoomChatOut, ZoomChatReset};

    use crate::test_support::{ShellRig, open_chat, resync, shell_rig, synced_chat};

    fn register(
        rig: &mut ShellRig,
        pane: protocol::PaneId,
    ) {
        let register = protocol::PaneFrame::Register {
            type_id: crate::panes::kinds::core_panes::chat_type().unwrap(),
            sdk: protocol::SDK_VERSION,
        };
        let text = protocol::encode(&register).unwrap();
        rig.shell.handle_ipc(panehost::IpcMessage { pane, text });
    }

    fn told(
        rig: &ShellRig,
        percent: u16,
    ) -> usize {
        let frame = format!(r#"{{\"sdk\":1,\"kind\":\"chat.zoom\",\"percent\":{percent}}}"#);
        rig.mock
            .calls()
            .into_iter()
            .filter(|call| matches!(call, webview::Call::Eval { script, .. } if script.contains(&frame)))
            .count()
    }

    #[test]
    fn the_chat_zoom_steps_within_develops_range() {
        let cases: [(&[keymap::ShellCommand], u16); 4] = [
            (&[ZoomChatIn, ZoomChatIn], 120),
            (&[ZoomChatOut; 9], 30),
            (&[ZoomChatIn; 25], 300),
            (&[ZoomChatIn, ZoomChatReset], 100),
        ];
        for (commands, percent) in cases {
            assert_eq!(zoomed_by(commands), percent, "{commands:?}");
        }
    }

    fn zoomed_by(commands: &[keymap::ShellCommand]) -> u16 {
        let mut rig = shell_rig();
        for command in commands {
            rig.shell.zoom_chat(*command);
        }
        rig.shell.host.chat_zoom()
    }

    #[test]
    fn a_chat_opened_after_a_zoom_shows_it_as_the_first_did() {
        let mut rig = shell_rig();
        let first = synced_chat(&mut rig);
        register(&mut rig, first.clone());
        rig.shell.zoom_chat(ZoomChatIn);
        open_chat(&mut rig.shell, "t2");
        let (tab, second) = rig
            .shell
            .layout()
            .tabs()
            .iter()
            .flat_map(|tab| tab.pane_ids().into_iter().map(|pane| (tab.id(), pane)))
            .find(|(_, pane)| *pane != first)
            .expect("the second chat opened");
        rig.shell.apply(vec![layout::Command::ActivateTab { tab }]);
        resync(&mut rig);
        register(&mut rig, second);
        assert_eq!(told(&rig, 110), 2, "{:?}", rig.mock.calls());
    }
}
