//! Asks the shell's own confirmations as the desktop's message dialog, over the shell window.

use super::ShellView;
use crate::shell::inbox::request_queue::{Requests, send};
use crate::shell::inbox::shell_request::ShellRequest;

impl ShellView {
    pub(super) fn ask_natively(
        &self,
        request: webview::AlertRequest,
        answer: impl FnOnce(Option<usize>) -> Option<ShellRequest> + Send + 'static,
    ) -> bool {
        let Some((parent, _)) = self.pane_area.placement else {
            return false;
        };
        let requests = self.requests.clone();
        let answered: webview::ChosenButton =
            Box::new(move |pressed| forward(&requests, answer(pressed)));
        let asked = self.state.host().desktop().ask(parent, request, answered);
        if let Err(error) = &asked {
            tracing::debug!(%error, "the shell draws its own dialog");
        }
        asked.is_ok()
    }
}

fn forward(
    requests: &Requests,
    request: Option<ShellRequest>,
) {
    if let Some(request) = request {
        send(requests, request);
    }
}

#[cfg(test)]
mod tests {
    fn saves_asked(mock: &webview::MockPlatform) -> usize {
        let asks = |call: &&webview::Call| matches!(call, webview::Call::Eval { script, .. } if script.contains("pane.save"));
        mock.calls().iter().filter(asks).count()
    }

    fn alerts_asked(mock: &webview::MockPlatform) -> Vec<Option<String>> {
        let title = |call: &webview::Call| match call {
            webview::Call::AskAlert { request, .. } => Some(request.title.clone()),
            _ => None,
        };
        mock.calls().iter().filter_map(title).collect()
    }

    #[gpui::test]
    fn closing_with_unsaved_work_asks_the_desktop_s_dialog_whose_save_saves(
        cx: &mut gpui::TestAppContext
    ) {
        let mut opened = crate::shell::shell_view::tests::open_shell(cx);
        let pane = opened.pane.clone();
        opened.mock.press_button(2);
        let area = webview::Bounds {
            x: 0.0,
            y: 0.0,
            width: 900.0,
            height: 700.0,
        };
        let closes = opened.shell.update(&mut opened.window, |shell, cx| {
            shell.pane_area.placement = Some((webview::ParentWindow::X11(9), area));
            shell.state.mark_dirty(pane);
            shell.allow_window_close(cx)
        });
        let before = saves_asked(&opened.mock);
        opened.window.run_until_parked();
        assert!(!closes, "the window waits for the answer");
        assert_eq!(
            alerts_asked(&opened.mock),
            [Some("Unsaved Changes".to_owned())]
        );
        assert!(
            opened
                .shell
                .read_with(&opened.window, |shell, _| shell.question.is_none())
        );
        assert_eq!(
            saves_asked(&opened.mock) - before,
            1,
            "Save asks the page to save"
        );
    }
}
