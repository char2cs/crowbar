use gpui::{AppContext as _, Context, Entity, Window};
use layout::Command;
use protocol::{PaneId, ShellFrame};

use super::ShellView;
use crate::close_choice::CloseChoice;
use crate::close_step::CloseStep;
use crate::closing::Closing;
use crate::panes::editor::document_request::DocumentRequest;
use crate::panes::ide::tab_looks::tab_look;
use crate::panes::tracking::close_guard::dirty_among;
use crate::panes::tracking::titles::Titles;
use crate::pending_close::PendingClose;
use crate::request_queue::send;
use crate::shell_request::ShellRequest;
use crate::unsaved_dialog::{self, UnsavedDialog};

pub(super) fn names_of(
    panes: &[PaneId],
    known: (&layout::Layout, &Titles),
) -> Vec<String> {
    panes.iter().map(|pane| name_of(pane, known)).collect()
}

fn name_of(
    pane: &PaneId,
    (layout, titles): (&layout::Layout, &Titles),
) -> String {
    let title = titles.get(pane).map(|title| title.to_string());
    let Some(instance) = layout.pane(pane) else {
        return title.unwrap_or_else(|| pane.to_string());
    };
    tab_look(instance, title.as_deref()).label.to_string()
}

impl ShellView {
    pub(super) fn apply_guarded(
        &mut self,
        commands: Vec<Command>,
        cx: &mut Context<'_, Self>,
    ) {
        let flags = self.state.dirty();
        let dirty = dirty_among(self.state.layout(), &commands, &flags);
        if dirty.is_empty() {
            self.state.apply(commands);
            self.refresh(cx);
            return;
        }
        self.ask_before_closing(PendingClose::new(Closing::Panes(commands), dirty), cx);
    }

    pub(crate) fn allow_window_close(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) -> bool {
        let dirty = self.state.dirty().all();
        if dirty.is_empty() {
            self.state.close_web_views();
            return true;
        }
        if self.pending_close.is_none() {
            self.ask_before_closing(PendingClose::new(Closing::Window, dirty), cx);
        }
        false
    }

    fn ask_before_closing(
        &mut self,
        pending: PendingClose,
        cx: &mut Context<'_, Self>,
    ) {
        let names = names_of(pending.dirty(), (self.state.layout(), &self.state.titles()));
        self.pending_close = Some(pending);
        cx.notify();
        let request = unsaved_dialog::native_request(&names);
        let choice = |pressed| Some(ShellRequest::Closing(unsaved_dialog::choice_of(pressed)));
        if self.ask_natively(request, choice) {
            return;
        }
        let dialog = cx.new(|_| UnsavedDialog::new(&names));
        self.subscriptions.push(
            cx.subscribe(&dialog, |this, source, choice: &CloseChoice, cx| {
                this.on_unsaved_choice(source, *choice, cx);
            }),
        );
        self.question = Some(dialog.into());
    }

    fn on_unsaved_choice(
        &mut self,
        _: Entity<UnsavedDialog>,
        choice: CloseChoice,
        _: &mut Context<'_, Self>,
    ) {
        send(&self.requests, ShellRequest::Closing(choice));
    }

    pub(super) fn choose_closing(
        &mut self,
        choice: CloseChoice,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.question = None;
        let Some(pending) = self.pending_close.take() else {
            return cx.notify();
        };
        if choice == CloseChoice::Save {
            self.request_saves(pending.dirty());
        }
        let step = pending.choose(choice, &self.state.dirty());
        self.advance(step, window, cx);
    }

    pub(super) fn resume_closing(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(pending) = self.pending_close.take() else {
            return;
        };
        let step = pending.settle(&self.state.dirty());
        self.advance(step, window, cx);
    }

    fn advance(
        &mut self,
        step: CloseStep<PendingClose>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match step {
            CloseStep::Run(Closing::Panes(commands)) => {
                self.state.apply(commands);
                self.refresh(cx);
            },
            CloseStep::Run(Closing::Window) => {
                self.state.close_web_views();
                window.remove_window();
            },
            CloseStep::Wait(pending) => self.pending_close = Some(pending),
            CloseStep::Stop => {},
        }
        cx.notify();
    }

    pub(super) fn request_saves(
        &self,
        panes: &[PaneId],
    ) {
        for pane in panes {
            self.request_save(pane);
        }
    }

    fn request_save(
        &self,
        pane: &PaneId,
    ) {
        if self.source.native().editors().contains(pane) {
            return send(
                &self.requests,
                ShellRequest::Document(DocumentRequest::Save { pane: pane.clone() }),
            );
        }
        if let Err(error) = self.state.host().send(pane, &ShellFrame::Save) {
            tracing::warn!(%error, %pane, "asking a pane to save failed");
        }
    }
}

#[cfg(test)]
mod tests {
    use protocol::PaneTitle;

    use super::*;

    fn saves_asked(mock: &webview::MockPlatform) -> usize {
        let asks = |call: &&webview::Call| matches!(call, webview::Call::Eval { script, .. } if script.contains("pane.save"));
        mock.calls().iter().filter(asks).count()
    }

    #[gpui::test]
    fn ctrl_shift_s_asks_every_unsaved_pane_to_save(cx: &mut gpui::TestAppContext) {
        let mut opened = crate::shell_view::tests::open_shell(cx);
        let pane = opened.pane.clone();
        opened
            .shell
            .update(&mut opened.window, |shell, _| shell.state.mark_dirty(pane));
        let before = saves_asked(&opened.mock);
        opened
            .window
            .simulate_keystrokes(&crate::shell_view::test_support::keys_of(
                keymap::ShellCommand::SaveAll,
            ));
        opened.window.run_until_parked();
        assert_eq!(saves_asked(&opened.mock) - before, 1, "the dirty chat page");
    }

    fn web_views(opened: &mut crate::shell_view::tests::Opened) -> usize {
        opened.shell.update(&mut opened.window, |shell, _| {
            shell.state.host().web_view_count()
        })
    }

    #[gpui::test]
    fn a_window_allowed_to_close_has_destroyed_its_web_views(cx: &mut gpui::TestAppContext) {
        let mut opened = crate::shell_view::tests::open_shell(cx);
        assert!(web_views(&mut opened) > 0, "the chat page");
        let allowed = opened
            .shell
            .update(&mut opened.window, ShellView::allow_window_close);
        assert!(allowed);
        assert_eq!(
            web_views(&mut opened),
            0,
            "gone while their parent window still exists"
        );
    }

    #[test]
    fn a_pane_is_named_as_its_tab_is_and_by_its_id_when_it_has_no_tab() {
        let mut layout = layout::Layout::default();
        let serials = crate::panes::kinds::serials::Serials::above(&layout);
        let file =
            crate::panes::kinds::pane_spec::PaneSpec::editor("c1", "src/pricing.ts").unwrap();
        for command in file.open_commands((&layout, ""), &serials).unwrap() {
            layout.apply(command).unwrap();
        }
        let editor = layout.tabs()[0].pane_ids()[0].clone();
        let titles = Titles::default();
        let (titled, gone) = (PaneId::new("p7").unwrap(), PaneId::new("p8").unwrap());
        titles.set(titled.clone(), PaneTitle::new("a.rs").unwrap());
        let names = names_of(&[editor, titled, gone], (&layout, &titles));
        assert_eq!(names, ["pricing.ts", "a.rs", "p8"]);
    }
}
