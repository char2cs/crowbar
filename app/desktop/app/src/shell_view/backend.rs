use std::rc::Rc;
use std::time::Duration;

use gpui::{AppContext as _, AsyncApp, Context, Entity, WeakEntity};
use ui::{ToastContent, ToastLifetime};

use super::ShellView;
use crate::backend::{
    BackendStatus, BackendWatch, ConnectionNotice, CrashPanel, NoticeAct, backend_status,
    reports_answer,
};
use crate::workspace::Workspace;

const CONNECTION_KEY: &str = "connection-indicator";
const CRASH_KEY: &str = "daemon-crash-panel";
pub(super) const SHOW_DELAY: Duration = Duration::from_millis(500);

impl ShellView {
    pub(super) fn follow_backend(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let event = self.link.current_event();
        let link = self.data.local_link();
        if reports_answer(event.as_ref(), link) {
            self.link.heard_answer();
        }
        let status = backend_status(event.as_ref(), link);
        self.follow_crash(&status, cx);
        let act = self.backend.connection.follow(status);
        if act != NoticeAct::Keep {
            self.backend.warning = None;
        }
        match act {
            NoticeAct::Keep | NoticeAct::Settle => {},
            NoticeAct::Dismiss => self.clear_keyed(CONNECTION_KEY, cx),
            NoticeAct::Show(content, lifetime) => {
                self.show_keyed(CONNECTION_KEY, (content, lifetime), cx);
            },
            NoticeAct::Renew(content, lifetime) => {
                self.clear_keyed(CONNECTION_KEY, cx);
                self.show_keyed(CONNECTION_KEY, (content, lifetime), cx);
            },
            NoticeAct::WarnLater => {
                self.backend.warning =
                    Some(cx.spawn(async move |this, cx| warn_later(this, cx).await));
            },
        }
        cx.notify();
    }

    fn follow_crash(
        &mut self,
        status: &BackendStatus,
        cx: &mut Context<'_, Self>,
    ) {
        let BackendStatus::CrashLoop(report) = status else {
            self.backend.crash = None;
            return;
        };
        if self.backend.crash.is_some() {
            return;
        }
        let link = Rc::clone(&self.link);
        let panel = cx.new(|_| CrashPanel::new(report.clone(), link));
        let toasts = cx.subscribe(&panel, |shell, _, toast: &ToastContent, cx| {
            shell.show_keyed(CRASH_KEY, (toast.clone(), ToastLifetime::Auto), cx);
        });
        self.backend.crash = Some((panel, toasts));
    }

    fn warn(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let warning = self.backend.connection.warn();
        self.show_keyed(CONNECTION_KEY, warning, cx);
    }
}

pub(super) fn watching(workspace: Entity<Workspace>) -> BackendWatch {
    BackendWatch {
        connection: ConnectionNotice::default(),
        warning: None,
        crash: None,
        workspace,
    }
}

async fn warn_later(
    shell: WeakEntity<ShellView>,
    cx: &mut AsyncApp,
) {
    cx.background_executor().timer(SHOW_DELAY).await;
    if shell.update(cx, ShellView::warn).is_err() {
        tracing::debug!("the shell closed before its backend warning");
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use layout::{Command, Tab};

    use super::SHOW_DELAY;
    use crate::shell_view::ShellView;
    use crate::shell_view::tests::{Opened, open_shell, open_shell_unsettled};

    fn shown(opened: &mut Opened) -> usize {
        opened.window.run_until_parked();
        opened
            .shell
            .update(&mut opened.window, |shell, cx| shell.notices.read(cx).len())
    }

    #[gpui::test]
    fn a_lost_daemon_is_announced_only_once_it_has_stayed_lost_a_moment(cx: &mut TestAppContext) {
        let mut opened = open_shell_unsettled(cx);
        let before = shown(&mut opened);
        opened
            .shell
            .update(&mut opened.window, ShellView::follow_backend);
        assert_eq!(shown(&mut opened), before, "a blip shows nothing");
        opened.window.executor().advance_clock(SHOW_DELAY);
        assert_eq!(
            shown(&mut opened),
            before + 1,
            "Backend unavailable / Reconnecting…"
        );
    }

    const RETRY_SLOT: &str = "projects-retry-slot";

    fn retry_slot(opened: &mut Opened) -> Option<gpui::Bounds<gpui::Pixels>> {
        opened.window.run_until_parked();
        opened.window.debug_bounds(RETRY_SLOT)
    }

    fn fail_projects(opened: &mut Opened) {
        opened.shell.update(&mut opened.window, |shell, _| {
            shell.data.lose_projects_snapshot();
            shell.data.lose_projects_snapshot();
        });
        let load = opened
            .shell
            .update(&mut opened.window, |shell, _| shell.data.projects_load());
        assert_eq!(load, store::ProjectsLoad::Failed);
    }

    fn has_tabs(opened: &mut Opened) -> bool {
        opened.shell.update(&mut opened.window, |shell, cx| {
            shell.tabs.read(cx).has_tabs()
        })
    }

    fn stage_bottom(opened: &mut Opened) -> Option<gpui::Pixels> {
        opened
            .window
            .debug_bounds("workspace-stage")
            .map(|stage| stage.bottom())
    }

    #[gpui::test]
    fn a_failed_projects_load_shows_the_landing_below_the_open_tabs(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        fail_projects(&mut opened);
        assert!(has_tabs(&mut opened));
        let slot = retry_slot(&mut opened);
        assert!(
            slot.is_some(),
            "develop's landing shows under restored panes"
        );
        let slot = slot.unwrap();
        let bottom = stage_bottom(&mut opened);
        assert!(
            bottom.is_some_and(|bottom| slot.top() >= bottom),
            "the panes give the landing the lower stage: {slot:?} under {bottom:?}"
        );
    }

    #[gpui::test]
    fn closing_the_last_tab_after_the_projects_failed_keeps_the_landing(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        fail_projects(&mut opened);
        opened.shell.update(&mut opened.window, |shell, cx| {
            let panes = shell.state.layout().tabs().iter().flat_map(Tab::pane_ids);
            let closes = panes.map(|pane| Command::ClosePane { pane }).collect();
            shell.apply_guarded(closes, cx);
        });
        assert!(!has_tabs(&mut opened), "the last tab closed");
        assert!(
            retry_slot(&mut opened).is_some(),
            "the empty stage lands on the failure"
        );
    }
}
