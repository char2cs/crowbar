use std::rc::Rc;
use std::time::Duration;

use gpui::{AppContext as _, AsyncApp, Context, Entity, WeakEntity};
use store::ProjectsLoad;
use ui::{ToastContent, ToastLifetime};

use super::ShellView;
use crate::backend::{
    BackendStatus, BackendWatch, ConnectionNotice, CrashPanel, NoticeAct, ProjectsFailed,
    backend_status,
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
        let status = backend_status(self.link.current_event().as_ref(), self.data.local_link());
        self.follow_crash(&status, cx);
        self.follow_projects(cx);
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

    fn follow_projects(
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

    use super::SHOW_DELAY;
    use crate::shell_view::ShellView;
    use crate::shell_view::tests::{Opened, open_shell_unsettled};

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
}
