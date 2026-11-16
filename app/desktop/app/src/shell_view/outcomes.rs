use gpui::Context;
use protocol::CommandId;
use shell_keys::ShellCommand;

use super::ShellView;
use crate::outcome::Outcome;
use crate::request_queue::send;
use crate::shell_request::ShellRequest;

impl ShellView {
    pub(super) fn apply_outcome(
        &mut self,
        outcome: Outcome,
        cx: &mut Context<'_, Self>,
    ) {
        let Outcome {
            titles_changed,
            hover,
            commands,
            pages,
            settings,
        } = outcome;
        self.store_pane_settings(settings);
        for (pane, address) in pages {
            self.source.show_address(&pane, &address, cx);
        }
        for (pane, hovered) in hover {
            let entered = hovered.then(|| self.hover.entered(pane.clone()));
            let current = entered.unwrap_or_else(|| self.hover.left(&pane));
            self.layout
                .update(cx, |view, cx| view.set_hovered_pane(current, cx));
        }
        for command in served_commands(&commands) {
            self.queue_command(command);
        }
        if titles_changed {
            self.refresh(cx);
        }
    }

    fn queue_command(
        &self,
        command: ShellCommand,
    ) {
        send(&self.requests, ShellRequest::Command(command));
    }
}

fn served_commands(ids: &[CommandId]) -> Vec<ShellCommand> {
    ids.iter()
        .filter_map(|id| ShellCommand::parse(id.as_str()))
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_the_commands_the_shell_serves_are_kept_in_order() {
        let ids = [
            "navigation.toggleSidebar",
            "extension.custom",
            "navigation.toggleConsole",
        ]
        .map(|id| CommandId::new(id).unwrap());
        assert_eq!(
            served_commands(&ids),
            [ShellCommand::ToggleSidebar, ShellCommand::ToggleConsole],
            "unknown ids are dropped"
        );
    }
}
