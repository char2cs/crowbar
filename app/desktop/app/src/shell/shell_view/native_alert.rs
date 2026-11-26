//! Asks the shell's confirmations as the desktop's message dialog over the shell window; when
//! it cannot be asked the question counts as cancelled.

use super::ShellView;
use crate::shell::inbox::request_queue::{Requests, send};
use crate::shell::inbox::shell_request::ShellRequest;

impl ShellView {
    pub(super) fn ask_natively(
        &self,
        request: webview::AlertRequest,
        answer: impl FnOnce(Option<usize>) -> Option<ShellRequest> + Clone + Send + 'static,
    ) {
        let Some((parent, _)) = self.pane_area.placement else {
            return self.cancel(&"no window to ask over", answer);
        };
        let (requests, cancel) = (self.requests.clone(), answer.clone());
        let answered: webview::ChosenButton =
            Box::new(move |pressed| forward(&requests, answer(pressed)));
        if let Err(error) = self.state.host().desktop().ask(parent, request, answered) {
            self.cancel(&error, cancel);
        }
    }

    fn cancel(
        &self,
        why: &dyn std::fmt::Display,
        answer: impl FnOnce(Option<usize>) -> Option<ShellRequest>,
    ) {
        tracing::error!(%why, "the message dialog could not be asked; cancelled");
        forward(&self.requests, answer(None));
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
    fn closing_with_unsaved_work_and_no_window_to_ask_over_stays_open_and_cancels(
        cx: &mut gpui::TestAppContext
    ) {
        let mut opened = crate::shell::shell_view::tests::open_shell(cx);
        let pane = opened.pane.clone();
        let closes = opened.shell.update(&mut opened.window, |shell, cx| {
            shell.pane_area.placement = None;
            shell.state.mark_dirty(pane);
            shell.allow_window_close(cx)
        });
        opened.window.run_until_parked();
        assert!(!closes, "unsaved work keeps the window");
        let pending = opened
            .shell
            .read_with(&opened.window, |shell, _| shell.pending_close.is_some());
        assert!(!pending, "the question counts as cancelled");
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
        assert_eq!(
            saves_asked(&opened.mock) - before,
            1,
            "Save asks the page to save"
        );
    }
}
