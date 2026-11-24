use gpui::{App, Context, Window};
use ui::{ToastContent, ToastKind, ToastLifetime, ToastPlacement};

use super::ShellView;
use crate::backend::daemon_notice::{DaemonNotice, daemon_notice};
use crate::notice_key::NoticeKey;
use crate::save_report::SaveReport;

impl ShellView {
    pub(super) fn track_daemon(
        &mut self,
        input: &startup::Input,
        cx: &mut Context<'_, Self>,
    ) {
        match daemon_notice(input) {
            DaemonNotice::Nothing => {},
            DaemonNotice::Show(key, content) => self.show_toast(key, content, cx),
        }
        self.follow_backend(cx);
    }

    pub(super) fn announce_start(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(text) = self.state.take_notice() else {
            return;
        };
        self.show_notice(NoticeKey::Layout, (ToastKind::Warning, text), cx);
    }

    pub(super) fn on_save_report(
        &mut self,
        report: SaveReport,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match report {
            SaveReport::Failed(reason) => {
                let text = format!("Your layout could not be saved: {reason}");
                self.show_notice(NoticeKey::Storage, (ToastKind::Error, text), cx);
            },
            SaveReport::Recovered => self.clear_notice(NoticeKey::Storage, cx),
        }
    }

    pub(super) fn show_notice(
        &mut self,
        key: NoticeKey,
        (kind, text): (ToastKind, String),
        cx: &mut Context<'_, Self>,
    ) {
        self.show_toast(key, ToastContent::new(kind, text), cx);
    }

    pub(super) fn show_toast(
        &self,
        key: NoticeKey,
        content: ToastContent,
        cx: &mut Context<'_, Self>,
    ) {
        self.show_keyed(key.slug(), (content, key.lifetime()), cx);
    }

    pub(super) fn show_keyed(
        &self,
        key: &str,
        (content, lifetime): (ToastContent, ToastLifetime),
        cx: &mut Context<'_, Self>,
    ) {
        self.notices.update(cx, |queue, cx| {
            queue.show(key.to_owned(), (content, lifetime), cx);
        });
    }

    pub(super) fn clear_notice(
        &mut self,
        key: NoticeKey,
        cx: &mut Context<'_, Self>,
    ) {
        self.clear_keyed(key.slug(), cx);
    }

    pub(super) fn clear_keyed(
        &self,
        key: &str,
        cx: &mut Context<'_, Self>,
    ) {
        self.notices.update(cx, |queue, cx| queue.clear(key, cx));
    }

    pub(super) fn floats_toasts(
        &self,
        cx: &App,
    ) -> bool {
        let queue = self.notices.read(cx);
        !queue.is_empty() && matches!(queue.placement(), ToastPlacement::Floating(_))
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use gpui::TestAppContext;

    use crate::notice_key::NoticeKey;
    use crate::shell_view::tests::open_shell;

    #[gpui::test]
    fn a_notice_outlives_hiding_the_sidebar_and_floats_over_the_window_meanwhile(
        cx: &mut TestAppContext
    ) {
        let mut opened = open_shell(cx);
        let floats = opened
            .shell
            .update_in(&mut opened.window, |shell, window, cx| {
                let text = (
                    "The layout could not be saved".to_owned(),
                    NoticeKey::Storage,
                );
                shell.show_notice(text.1, (ui::ToastKind::Error, text.0), cx);
                let docked = shell.floats_toasts(cx);
                shell
                    .sidebar
                    .update(cx, |sidebar, cx| sidebar.toggle(window, cx));
                (docked, shell.floats_toasts(cx))
            });
        assert_eq!(
            floats,
            (false, true),
            "docked in the open sidebar, then floating"
        );
    }

    #[gpui::test]
    fn a_notice_waits_while_the_window_is_inactive_and_runs_out_once_it_is_active_again(
        cx: &mut TestAppContext
    ) {
        let mut opened = open_shell(cx);
        let count = |opened: &mut crate::shell_view::tests::Opened| {
            opened
                .shell
                .read_with(&opened.window, |shell, cx| shell.notices.read(cx).len())
        };
        opened.window.update(|window, _| window.activate_window());
        opened.window.run_until_parked();
        let base = count(&mut opened);
        opened.shell.update(&mut opened.window, |shell, cx| {
            let text = ("The command failed".to_owned(), NoticeKey::Command);
            shell.show_notice(text.1, (ui::ToastKind::Error, text.0), cx);
        });
        opened.window.deactivate_window();
        opened.window.run_until_parked();
        opened
            .window
            .executor()
            .advance_clock(Duration::from_secs(60));
        assert_eq!(count(&mut opened), base + 1, "held while nobody looks");
        opened.window.update(|window, _| window.activate_window());
        opened.window.run_until_parked();
        opened
            .window
            .executor()
            .advance_clock(Duration::from_secs(60));
        opened.window.executor().advance_clock(ui::timing::SLOW);
        assert_eq!(
            count(&mut opened),
            base,
            "ran out once the window was active"
        );
    }
}
