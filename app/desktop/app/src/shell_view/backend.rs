use std::rc::Rc;
use std::time::Duration;

use gpui::{AppContext as _, AsyncApp, Context, Entity, WeakEntity};
use store::ProjectsLoad;
use ui::{ToastContent, ToastLifetime};

use super::ShellView;
use crate::backend::{
    BackendStatus, BackendWatch, ConnectionNotice, CrashPanel, NoticeAct, ProjectsFailed,
    backend_status, reports_answer,
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

    pub(super) fn follow_projects(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let failed =
            self.data.projects_load() == ProjectsLoad::Failed && !self.tabs.read(cx).has_tabs();
        if failed == self.backend.workspace.read(cx).has_landing() {
            return;
        }
        let data = Rc::clone(&self.data);
        let landing =
            failed.then(|| cx.new(|_| ProjectsFailed::new(Rc::new(move || data.retry_projects()))));
        self.backend
            .workspace
            .update(cx, |workspace, cx| workspace.show_landing(landing, cx));
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

    fn landing(opened: &mut Opened) -> bool {
        opened.window.run_until_parked();
        opened.shell.update(&mut opened.window, |shell, cx| {
            shell.backend.workspace.read(cx).has_landing()
        })
    }

    fn has_tabs(opened: &mut Opened) -> bool {
        opened.shell.update(&mut opened.window, |shell, cx| {
            shell.tabs.read(cx).has_tabs()
        })
    }

    #[gpui::test]
    fn closing_the_last_tab_after_the_projects_failed_shows_the_landing(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        opened.shell.update(&mut opened.window, |shell, _| {
            shell.data.lose_projects_snapshot();
            shell.data.lose_projects_snapshot();
        });
        let load = opened
            .shell
            .update(&mut opened.window, |shell, _| shell.data.projects_load());
        assert_eq!(load, store::ProjectsLoad::Failed);
        assert!(has_tabs(&mut opened));
        assert!(!landing(&mut opened), "an open tab keeps the stage");
        opened.shell.update(&mut opened.window, |shell, cx| {
            let panes = shell.state.layout().tabs().iter().flat_map(Tab::pane_ids);
            let closes = panes.map(|pane| Command::ClosePane { pane }).collect();
            shell.apply_guarded(closes, cx);
        });
        assert!(!has_tabs(&mut opened), "the last tab closed");
        assert!(landing(&mut opened), "the empty stage lands on the failure");
    }
}
