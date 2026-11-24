use gpui::Context;
use keymap::ShellCommand;
use layout::Command;
use protocol::{CommandId, PaneId};

use super::ShellView;
use crate::document_request::DocumentRequest;
use crate::outcome::Outcome;
use crate::request_queue::send;
use crate::review_panes::ViewedChanges;
use crate::shell_request::ShellRequest;

type ViewedPanes = Vec<(PaneId, ViewedChanges)>;

impl ShellView {
    fn focus_pages(
        &mut self,
        focused: Vec<PaneId>,
        cx: &mut Context<'_, Self>,
    ) {
        for pane in focused {
            self.apply_guarded(vec![Command::FocusPane { pane }], cx);
        }
    }

    fn follow_pages(
        &mut self,
        (focused, opens): (Vec<PaneId>, Vec<Command>),
        cx: &mut Context<'_, Self>,
    ) {
        self.focus_pages(focused, cx);
        self.open_for_pages(opens, cx);
    }

    fn open_for_pages(
        &mut self,
        opens: Vec<Command>,
        cx: &mut Context<'_, Self>,
    ) {
        if opens.is_empty() {
            return;
        }
        self.state.apply(opens);
        self.refresh(cx);
    }

    fn serve_mounted(
        &mut self,
        mounted: &[PaneId],
        (documents, viewed): (Vec<DocumentRequest>, ViewedPanes),
        cx: &mut Context<'_, Self>,
    ) {
        self.serve_page_documents(mounted, documents, cx);
        self.serve_review_viewed(mounted, viewed);
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
            opens,
            commands,
            pages,
            loaded,
            picks,
            settings,
            mounted,
            viewed,
            documents,
            ..
        } = outcome;
        self.serve_mounted(&mounted, (documents, viewed), cx);
        self.store_pane_settings(settings);
        self.finish_picks(picks, cx);
        self.show_addresses(pages, cx);
        for (pane, hovered) in hover {
            let entered = hovered.then(|| self.hover.entered(pane.clone()));
            let current = entered.unwrap_or_else(|| self.hover.left(&pane));
            self.layout
                .update(cx, |view, cx| view.set_hovered_pane(current, cx));
        }
        self.follow_pages((focused, opens), cx);
        self.serve(&commands);
        if dirty_changed {
            send(&self.requests, ShellRequest::ResumeClosing);
        }
        if titles_changed {
            self.refresh(cx);
        }
        self.state.publish_tabs();
        self.run_deferred_agents(&loaded, cx);
    }

    fn run_deferred_agents(
        &mut self,
        loaded: &[PaneId],
        cx: &mut Context<'_, Self>,
    ) {
        for pane in loaded {
            self.run_deferred_agent(pane, cx);
        }
    }

    fn show_addresses(
        &self,
        pages: Vec<(PaneId, String)>,
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
        for command in commands
            .iter()
            .filter_map(|id| keymap::ContextualCommand::parse(id.as_str()))
        {
            send(&self.requests, ShellRequest::Contextual(command));
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
    use gpui::TestAppContext;
    use protocol::{DocumentPath, OpenMode, PaneFrame, PaneOpen, PaneTypeId, Params, encode};

    use super::*;
    use crate::core_panes::EDITOR;
    use crate::pane_spec::PaneSpec;
    use crate::shell_view::tests::{Opened, PLACED, open_shell};

    fn shows(call: &webview::Call) -> bool {
        let webview::Call::SetVisibility { visibility, .. } = call else {
            return false;
        };
        *visibility == webview::Visibility::Shown
    }

    fn shown_pages(opened: &Opened) -> usize {
        opened
            .mock
            .calls()
            .iter()
            .filter(|call| shows(call))
            .count()
    }

    fn open_files(
        shell: &mut ShellView,
        pane: &PaneId,
    ) {
        let layout = shell.state.layout();
        let chat = layout.pane(pane).and_then(crate::tab_looks::chat_of);
        let chat = chat.unwrap_or_default().to_owned();
        for file in ["src/a.ts", "src/pricing.ts"] {
            shell
                .state
                .open_spec(PaneSpec::editor(&chat, file).unwrap());
        }
    }

    fn with_files_open(cx: &mut TestAppContext) -> Opened {
        let mut opened = open_shell(cx);
        let pane = opened.pane.clone();
        opened.shell.update(&mut opened.window, |shell, cx| {
            open_files(shell, &pane);
            shell.placement = Some((webview::ParentWindow::X11(9), PLACED));
            shell.refresh(cx);
        });
        opened.window.run_until_parked();
        assert!(
            shown_pages(&opened) > 0,
            "the shown file's page is on screen"
        );
        opened
    }

    fn pages_shown_by_an_open(
        cx: &mut TestAppContext,
        (file, mode): (&str, OpenMode),
    ) -> usize {
        let mut opened = with_files_open(cx);
        let before = shown_pages(&opened);
        let frame = PaneFrame::Open(PaneOpen {
            type_id: PaneTypeId::new(EDITOR).unwrap(),
            params: Params::default(),
            file: Some(DocumentPath::new(file).unwrap()),
            mode,
        });
        let message = panehost::IpcMessage {
            pane: opened.pane.clone(),
            text: encode(&frame).unwrap(),
        };
        opened
            .shell
            .update_in(&mut opened.window, |shell, window, cx| {
                shell.on_ipc(message, window, cx);
            });
        opened.window.run_until_parked();
        shown_pages(&opened) - before
    }

    #[gpui::test]
    fn a_page_s_open_puts_the_file_on_screen_whether_new_or_already_open(cx: &mut TestAppContext) {
        let cases = [
            ("a new file, previewed", "src/b.ts", OpenMode::Preview),
            ("a new file, kept", "src/b.ts", OpenMode::Kept),
            ("an open file, previewed", "src/a.ts", OpenMode::Preview),
            ("an open file, kept", "src/a.ts", OpenMode::Kept),
        ];
        for (name, file, mode) in cases {
            assert!(pages_shown_by_an_open(cx, (file, mode)) > 0, "{name}");
        }
    }

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
