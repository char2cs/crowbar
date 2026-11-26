use gpui::{AppContext as _, Context, Entity};
use protocol::PaneId;

use crate::shell::dialogs::reload_dialog::{self, ReloadDialog};
use crate::shell::dialogs::unsaved::close_choice::CloseChoice;
use crate::shell::inbox::shell_request::ShellRequest;
use crate::shell::shell_view::ShellView;

impl ShellView {
    pub(crate) fn reload_tab(
        &mut self,
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) {
        if !self.state.dirty().contains(pane) {
            return self.source.native().editors().reload(pane, cx);
        }
        let name = self.source.native().editors().file_name(pane);
        let asked = (pane.clone(), name.unwrap_or_default());
        let request = reload_dialog::native_request(&asked.1);
        let discard = pane.clone();
        let reload = move |pressed| {
            (pressed == reload_dialog::OK).then_some(ShellRequest::ReloadDiscarding(discard))
        };
        if self.ask_natively(request, reload) {
            return;
        }
        let dialog = cx.new(|_| ReloadDialog::new(asked));
        self.subscriptions.push(
            cx.subscribe(&dialog, |this, dialog, choice: &CloseChoice, cx| {
                this.on_reload_choice(&dialog, *choice, cx);
            }),
        );
        self.question = Some(dialog.into());
        cx.notify();
    }

    fn on_reload_choice(
        &mut self,
        dialog: &Entity<ReloadDialog>,
        choice: CloseChoice,
        cx: &mut Context<'_, Self>,
    ) {
        self.question = None;
        cx.notify();
        if choice == CloseChoice::Discard {
            let pane = dialog.read(cx).pane().clone();
            self.source.native().editors().reload(&pane, cx);
        }
    }

    pub(crate) fn take_reloaded(
        &mut self,
        (pane, read): (&PaneId, (&str, editorview::SaveMark)),
        cx: &mut Context<'_, Self>,
    ) {
        let editors = self.source.native().editors();
        let took = editors.take_reloaded((pane, read), cx);
        self.disk_taken(pane, took, cx);
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::shell::shell_view::test_support::open_file;
    use crate::shell::shell_view::tests::open_shell;

    #[gpui::test]
    fn reloading_a_tab_with_unsaved_edits_asks_before_discarding_them(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        let pane = open_file(&mut opened, "src/pricing.ts");
        let event = layoutview::LayoutEvent::ReloadTab { pane: pane.clone() };
        let ask = |opened: &mut crate::shell::shell_view::tests::Opened| {
            let layout = opened
                .shell
                .read_with(&opened.window, |shell, _| shell.layout.clone());
            layout.update(&mut opened.window, |_, cx| cx.emit(event.clone()));
            opened.window.run_until_parked();
            opened
                .shell
                .read_with(&opened.window, |shell, _| shell.question.is_some())
        };
        assert!(!ask(&mut opened), "a clean tab reloads at once");
        opened.shell.update(&mut opened.window, |shell, _| {
            shell.state.mark_dirty(pane.clone());
        });
        assert!(ask(&mut opened), "a dirty tab asks first");
    }
}
