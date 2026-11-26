use gpui::{App, Context, Window};
use ui::{ToastContent, ToastKind, ToastLifetime, ToastPlacement};

use crate::daemon::backend::daemon_notice::{DaemonNotice, daemon_notice};
use crate::shell::shell_state::persistence::save_report::SaveReport;
use crate::shell::shell_view::ShellView;
use crate::shell::window::notice_key::NoticeKey;
use crate::shell::window::notices;

const ALERT_OK: &str = "OK";

impl ShellView {
    pub(crate) fn track_daemon(
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

    pub(crate) fn announce_start(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(text) = self.state.take_notice() else {
            return;
        };
        self.show_notice(NoticeKey::Layout, (ToastKind::Warning, text), cx);
    }

    pub(crate) fn on_save_report(
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

    pub(crate) fn show_notice(
        &mut self,
        key: NoticeKey,
        (kind, text): (ToastKind, String),
        cx: &mut Context<'_, Self>,
    ) {
        self.show_toast(key, ToastContent::new(kind, text), cx);
    }

    pub(crate) fn show_toast(
        &self,
        key: NoticeKey,
        content: ToastContent,
        cx: &mut Context<'_, Self>,
    ) {
        self.show_keyed(key.slug(), (content, key.lifetime()), cx);
    }

    pub(crate) fn show_refusal(
        &mut self,
        title: String,
        reason: String,
        cx: &mut Context<'_, Self>,
    ) {
        tracing::warn!(%title, %reason, "a command failed");
        let (key, notice) = notices::failure_notice(title, reason);
        self.show_keyed(&key, notice, cx);
    }

    pub(crate) fn show_keyed(
        &self,
        key: &str,
        (content, lifetime): (ToastContent, ToastLifetime),
        cx: &mut Context<'_, Self>,
    ) {
        self.notices.update(cx, |queue, cx| {
            queue.show(key.to_owned(), (content, lifetime), cx);
        });
    }

    pub(crate) fn clear_notice(
        &mut self,
        key: NoticeKey,
        cx: &mut Context<'_, Self>,
    ) {
        self.clear_keyed(key.slug(), cx);
    }

    pub(crate) fn clear_keyed(
        &self,
        key: &str,
        cx: &mut Context<'_, Self>,
    ) {
        self.notices.update(cx, |queue, cx| queue.clear(key, cx));
    }

    pub(crate) fn floats_toasts(
        &self,
        cx: &App,
    ) -> bool {
        let queue = self.notices.read(cx);
        !queue.is_empty() && matches!(queue.placement(), ToastPlacement::Floating(_))
    }
}

/// Tells of a failed command in the desktop's message dialog, with a lone OK.
pub(crate) fn show_alert(
    (title, message): (String, String),
    window: &Window,
    cx: &mut App,
) {
    tracing::warn!(%title, %message, "a command failed");
    let request = ui::AlertRequest {
        title: Some(title.into()),
        message: message.into(),
        buttons: vec![ui::AlertButton::new(ALERT_OK, ui::AlertRole::Accept)],
    };
    ui::NativeAlert::ask(request, window, cx).detach();
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use gpui::TestAppContext;

    use crate::shell::shell_view::tests::{open_shell, open_started_shell};
    use crate::shell::window::notice_key::NoticeKey;

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
    fn an_alert_asks_the_desktop_s_message_dialog_with_a_lone_ok(cx: &mut TestAppContext) {
        let asked = std::rc::Rc::new(std::cell::RefCell::new(Vec::new()));
        let record = std::rc::Rc::clone(&asked);
        let ask: ui::AskAlert = std::rc::Rc::new(move |request, _, _| {
            record.borrow_mut().push(request);
            gpui::Task::ready(Some(0))
        });
        let mut opened = open_shell(cx);
        opened
            .window
            .update(|_, cx| cx.set_global(ui::NativeAlert::new(ask)));
        let words = (
            "Move Failed".to_owned(),
            "Failed to move a.rs: locked".to_owned(),
        );
        opened
            .window
            .update(|window, cx| super::show_alert(words, window, cx));
        let request = ui::AlertRequest {
            title: Some("Move Failed".into()),
            message: "Failed to move a.rs: locked".into(),
            buttons: vec![ui::AlertButton::new("OK", ui::AlertRole::Accept)],
        };
        assert_eq!(*asked.borrow(), [request]);
    }

    #[gpui::test]
    fn a_notice_waits_while_the_window_is_inactive_and_runs_out_once_it_is_active_again(
        cx: &mut TestAppContext
    ) {
        let mut opened = open_started_shell(cx);
        let count = |opened: &mut crate::shell::shell_view::tests::Opened| {
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

    #[gpui::test]
    fn a_notice_runs_out_while_a_web_view_holds_the_window_s_keyboard(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        let count = |opened: &mut crate::shell::shell_view::tests::Opened| {
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
        assert_eq!(
            count(&mut opened),
            base + 1,
            "held while the keyboard is elsewhere"
        );
        opened.window.update(|window, cx| {
            ui::WindowActivity::gained(window.window_handle().window_id(), cx);
        });
        opened.window.run_until_parked();
        opened
            .window
            .executor()
            .advance_clock(Duration::from_secs(60) + ui::timing::SLOW);
        assert_eq!(
            count(&mut opened),
            base,
            "the keyboard stayed in the window"
        );
    }
}
