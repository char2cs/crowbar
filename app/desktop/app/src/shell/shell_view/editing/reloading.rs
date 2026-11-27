use gpui::Context;
use protocol::PaneId;

use crate::shell::dialogs::reload_dialog;
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
        let request = reload_dialog::native_request(&name.unwrap_or_default());
        let discard = pane.clone();
        let reload = move |pressed| {
            (pressed == reload_dialog::OK).then_some(ShellRequest::ReloadDiscarding(discard))
        };
        self.ask_natively(request, reload);
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

    use crate::shell::shell_view::tests::open_shell;

    const PATH: &str = "src/pricing.ts";

    #[gpui::test]
    fn reloading_a_tab_with_unsaved_edits_asks_before_discarding_them(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        let pane = opened.shell.update(&mut opened.window, |shell, cx| {
            shell.open_workspace_file(PATH, cx);
            let focused = shell.state.showing().and_then(layout::Tab::focused);
            focused.cloned().expect("the file pane has focus")
        });
        let event = layoutview::LayoutEvent::ReloadTab { pane: pane.clone() };
        let area = webview::Bounds {
            x: 0.0,
            y: 0.0,
            width: 900.0,
            height: 700.0,
        };
        opened.shell.update(&mut opened.window, |shell, _| {
            shell.pane_area.placement = Some((webview::ParentWindow::X11(9), area));
        });
        let ask = |opened: &mut crate::shell::shell_view::tests::Opened| {
            let layout = opened
                .shell
                .read_with(&opened.window, |shell, _| shell.layout.clone());
            layout.update(&mut opened.window, |_, cx| cx.emit(event.clone()));
            opened.window.run_until_parked();
            let asks = |call: &webview::Call| matches!(call, webview::Call::AskAlert { .. });
            opened.mock.calls().iter().any(asks)
        };
        assert!(!ask(&mut opened), "a clean tab reloads at once");
        opened.shell.update(&mut opened.window, |shell, _| {
            shell.state.mark_dirty(pane.clone());
        });
        assert!(ask(&mut opened), "a dirty tab asks first");
    }
}
