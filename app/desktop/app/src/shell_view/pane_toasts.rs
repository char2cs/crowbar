use gpui::Context;
use layout::Command;
use protocol::{DisplayText, PaneId};
use ui::{ToastContent, ToastKind, ToastLifetime};

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
            PaneToastChange::OpenRefused(pane, refusal) => self.refuse_open(pane, &refusal, cx),
        }
    }

    fn refuse_open(
        &mut self,
        pane: PaneId,
        (title, message): &(Option<DisplayText>, DisplayText),
        cx: &mut Context<'_, Self>,
    ) {
        let said = ToastContent::new(ToastKind::Error, message.to_string());
        let content = match title {
            Some(title) => said.titled(title.to_string()),
            None => said,
        };
        self.show_keyed(
            &format!("refused:{pane}"),
            (content, ToastLifetime::Auto),
            cx,
        );
        self.apply_guarded(vec![Command::ClosePane { pane }], cx);
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
    fn a_refused_open_closes_its_pane_and_says_why(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        let base = shown(&mut opened);
        let refused =
            r#"{"sdk":1,"kind":"pane.openRefused","title":"File not found","message":"src/a.ts"}"#;
        assert_eq!(post(&mut opened, refused), base + 1, "the toast stays");
        let pane = opened.pane.clone();
        let open = opened.shell.update(&mut opened.window, |shell, _| {
            shell.state.layout().pane(&pane).is_some()
        });
        assert!(!open, "the pane closed");
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
