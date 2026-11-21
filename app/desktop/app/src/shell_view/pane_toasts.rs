use gpui::Context;
use protocol::PaneId;

use super::ShellView;
use crate::notices::{pane_of_toast_key, pane_toast_content, pane_toast_key};
use crate::pane_toast_change::PaneToastChange;

impl ShellView {
    pub(super) fn show_pane_toasts(
        &mut self,
        changes: Vec<PaneToastChange>,
        cx: &mut Context<'_, Self>,
    ) {
        for change in changes {
            self.show_pane_toast(change, cx);
        }
    }

    fn show_pane_toast(
        &mut self,
        change: PaneToastChange,
        cx: &mut Context<'_, Self>,
    ) {
        match change {
            PaneToastChange::Shown(pane, toast) => {
                let key = pane_toast_key(&pane, toast.key.as_str());
                self.show_keyed(&key, pane_toast_content(&toast), cx);
            },
            PaneToastChange::Closed(pane, key) => {
                self.clear_keyed(&pane_toast_key(&pane, key.as_str()), cx);
            },
        }
    }

    pub(super) fn drop_toasts_of_closed_panes(
        &self,
        cx: &mut Context<'_, Self>,
    ) {
        let layout = self.state.layout();
        let closed = |key: &str| {
            let pane = pane_of_toast_key(key).and_then(|pane| PaneId::new(pane).ok());
            pane.is_some_and(|pane| layout.pane(&pane).is_none())
        };
        self.notices
            .update(cx, |queue, cx| queue.clear_matching(closed, cx));
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use layout::Command;

    use crate::shell_view::tests::{Opened, open_shell};

    fn post(
        opened: &mut Opened,
        text: &str,
    ) -> usize {
        let message = panehost::IpcMessage {
            pane: opened.pane.clone(),
            text: text.to_owned(),
        };
        opened
            .shell
            .update_in(&mut opened.window, |shell, window, cx| {
                shell.on_ipc(message, window, cx);
            });
        shown(opened)
    }

    fn shown(opened: &mut Opened) -> usize {
        opened.window.executor().advance_clock(ui::timing::SLOW);
        opened.window.run_until_parked();
        opened
            .shell
            .update(&mut opened.window, |shell, cx| shell.notices.read(cx).len())
    }

    const TOAST: &str =
        r#"{"sdk":1,"kind":"shell.toast","key":"t1","tone":"error","title":"Save the file"}"#;

    #[gpui::test]
    fn a_pane_toast_shows_in_the_shell_until_the_pane_closes_it(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        let base = shown(&mut opened);
        assert_eq!(post(&mut opened, TOAST), base + 1, "shown");
        assert_eq!(
            post(&mut opened, TOAST),
            base + 1,
            "rewritten under its key"
        );
        let close = r#"{"sdk":1,"kind":"shell.toastClose","key":"t1"}"#;
        assert_eq!(post(&mut opened, close), base, "closed by the pane");
    }

    #[gpui::test]
    fn closing_a_pane_takes_its_toasts_down(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        let base = shown(&mut opened);
        assert_eq!(post(&mut opened, TOAST), base + 1, "shown");
        let pane = opened.pane.clone();
        opened.shell.update(&mut opened.window, |shell, cx| {
            shell.apply_guarded(vec![Command::ClosePane { pane }], cx);
        });
        assert_eq!(shown(&mut opened), base, "gone with its pane");
    }
}
