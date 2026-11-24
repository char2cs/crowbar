use gpui::{AppContext as _, Context, Entity, Window};
use protocol::PaneId;

use super::ShellView;
use crate::close_choice::CloseChoice;
use crate::reload_dialog::ReloadDialog;

impl ShellView {
    pub(super) fn reload_tab(
        &mut self,
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) {
        if !self.state.dirty().contains(pane) {
            return self.source.native().editors().reload(pane, cx);
        }
        let name = self.source.native().editors().file_name(pane);
        let asked = (pane.clone(), name.unwrap_or_default());
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

    pub(super) fn take_reloaded(
        &mut self,
        (pane, read): (&PaneId, (&str, editorview::SaveMark)),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let editors = self.source.native().editors();
        let took = editors.take_reloaded((pane, read), window, cx);
        self.disk_taken(pane, took, cx);
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::shell_view::tests::open_shell;

    const PATH: &str = "src/pricing.ts";

    #[gpui::test]
    fn reloading_a_tab_with_unsaved_edits_asks_before_discarding_them(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        let pane = opened.shell.update(&mut opened.window, |shell, cx| {
            shell.open_workspace_file(PATH, cx);
            let layout = shell.state.layout();
            let focused = layout.active_tab().and_then(layout::Tab::focused);
            focused.cloned().expect("the file pane has focus")
        });
        let event = layoutview::LayoutEvent::ReloadTab { pane: pane.clone() };
        let ask = |opened: &mut crate::shell_view::tests::Opened| {
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
