use gpui::Context;
use keymap::ShellCommand;
use layout::Command;
use protocol::CommandId;

use super::ShellView;
use crate::outcome::Outcome;
use crate::request_queue::send;
use crate::shell_request::ShellRequest;

impl ShellView {
    fn focus_pages(
        &mut self,
        focused: Vec<protocol::PaneId>,
        cx: &mut Context<'_, Self>,
    ) {
        for pane in focused {
            self.apply_guarded(vec![Command::FocusPane { pane }], cx);
        }
    }

    pub(super) fn apply_outcome(
        &mut self,
        outcome: Outcome,
        cx: &mut Context<'_, Self>,
    ) {
        let Outcome {
            titles_changed,
            dirty_changed,
            hover,
            focused,
            dropped,
            commands,
            pages,
            loaded,
            picks,
            settings,
            mounted,
            viewed,
            ..
        } = outcome;
        self.serve_review_viewed(&mounted, viewed);
        self.store_pane_settings(settings);
        self.open_dropped(&dropped, cx);
        self.finish_picks(picks, cx);
        self.show_addresses(pages, cx);
        for (pane, hovered) in hover {
            let entered = hovered.then(|| self.hover.entered(pane.clone()));
            let current = entered.unwrap_or_else(|| self.hover.left(&pane));
            self.layout
                .update(cx, |view, cx| view.set_hovered_pane(current, cx));
        }
        self.focus_pages(focused, cx);
        self.serve(&commands);
        if dirty_changed {
            send(&self.requests, ShellRequest::ResumeClosing);
        }
        if titles_changed {
            self.refresh(cx);
        }
        self.state.publish_tabs();
        for pane in &loaded {
            self.run_deferred_agent(pane, cx);
        }
    }

    fn show_addresses(
        &self,
        pages: Vec<(protocol::PaneId, String)>,
        cx: &mut Context<'_, Self>,
    ) {
        for (pane, address) in pages {
            self.source.bars().show_address(&pane, &address, cx);
        }
    }

    fn serve(
        &self,
        commands: &[CommandId],
    ) {
        for command in served_commands(commands) {
            self.queue_command(command);
        }
        for slot in commands
            .iter()
            .filter_map(|id| keymap::project_slot_of_command(id.as_str()))
        {
            send(&self.requests, ShellRequest::SelectProject(slot));
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
