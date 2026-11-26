use gpui::Context;
use protocol::{DisplayText, PaneId};
use ui::{ToastContent, ToastKind, ToastLifetime};

use crate::panes::content::pane_toast_change::PaneToastChange;
use crate::shell::shell_view::ShellView;
use crate::shell::window::notices::{pane_of_toast_key, pane_toast_content, pane_toast_key};

impl ShellView {
    pub(crate) fn show_pane_toasts(
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
            PaneToastChange::OpenRefused(pane, refusal) => self.refuse_open(&pane, &refusal, cx),
        }
    }

    pub(crate) fn refuse_open(
        &mut self,
        pane: &PaneId,
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
        self.state.withdraw_open(pane);
        self.refresh(cx);
    }

    pub(crate) fn drop_toasts_of_closed_panes(
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

    use crate::shell::shell_view::test_support::{open_file, settled_notice_count};
    use crate::shell::shell_view::tests::{Opened, open_shell};

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
        settled_notice_count(opened)
    }

    const TOAST: &str =
        r#"{"sdk":1,"kind":"shell.toast","key":"t1","tone":"error","title":"Save the file"}"#;

    #[gpui::test]
    fn a_pane_toast_shows_in_the_shell_until_the_pane_or_its_closing_takes_it_down(
        cx: &mut TestAppContext
    ) {
        let mut opened = open_shell(cx);
        let base = settled_notice_count(&mut opened);
        assert_eq!(post(&mut opened, TOAST), base + 1, "shown");
        assert_eq!(
            post(&mut opened, TOAST),
            base + 1,
            "rewritten under its key"
        );
        let close = r#"{"sdk":1,"kind":"shell.toastClose","key":"t1"}"#;
        assert_eq!(post(&mut opened, close), base, "closed by the pane");
        assert_eq!(post(&mut opened, TOAST), base + 1, "shown again");
        let pane = opened.pane.clone();
        opened.shell.update(&mut opened.window, |shell, cx| {
            shell.apply_guarded(vec![Command::ClosePane { pane }], cx);
        });
        assert_eq!(
            settled_notice_count(&mut opened),
            base,
            "gone with its pane"
        );
    }

    #[gpui::test]
    fn a_refused_open_closes_its_pane_and_says_why(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        let base = settled_notice_count(&mut opened);
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
    fn a_refused_open_leaves_the_tab_that_was_active_before_it(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        let before = open_file(&mut opened, "src/a.ts");
        open_file(&mut opened, "src/b.ts");
        opened.shell.update(&mut opened.window, |shell, cx| {
            let pane = before.clone();
            shell.apply_guarded(vec![Command::FocusPane { pane }], cx);
        });
        opened.pane = open_file(&mut opened, "src/gone.ts");
        opened.shell.update(&mut opened.window, |shell, _| {
            crate::test_support::sync_shown(&mut shell.state);
        });
        let gone = (
            &opened.pane.clone(),
            Err(editor::OpenFailure::of("src/gone.ts", 404, None)),
        );
        opened.shell.update(&mut opened.window, |shell, cx| {
            shell.land_document(gone, cx);
        });
        let shown = opened.shell.update(&mut opened.window, |shell, _| {
            let tab = shell.state.showing().expect("a tab");
            let group = tab
                .group_of(&before)
                .and_then(layout::PaneGroup::active_tab);
            (tab.focused().cloned(), group.cloned())
        });
        assert_eq!(shown, (Some(before.clone()), Some(before)));
    }
}
