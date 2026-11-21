use gpui::Context;
use layout::Command;
use protocol::{PaneId, PaneState};
use ptysession::Report;
use ui::{ToastContent, ToastKind};

use super::ShellView;
use crate::notice_key::NoticeKey;
use crate::terminal_ids::SESSION_KEY;

const UNAVAILABLE: &str = "Terminal unavailable";
const TRY_AGAIN: &str = "Could not start the terminal. Try reopening the tab.";

impl ShellView {
    pub(super) fn on_terminal_session(
        &mut self,
        (pane, report): (&PaneId, Report),
        cx: &mut Context<'_, Self>,
    ) {
        match report {
            Report::Bound(session) => self.remember_session(pane, &session),
            Report::Gone => self.close_terminal(pane, cx),
            Report::Failed(reason) => {
                tracing::warn!(%reason, "a terminal could not start");
                let content = ToastContent::new(ToastKind::Error, TRY_AGAIN).titled(UNAVAILABLE);
                self.show_toast(NoticeKey::Terminal, content, cx);
            },
        }
    }

    pub(super) fn close_terminal(
        &mut self,
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) {
        if !self.source.native().closes_on_exit(pane) {
            return;
        }
        self.state.close_pane(pane.clone());
        self.refresh(cx);
    }

    fn remember_session(
        &mut self,
        pane: &PaneId,
        session: &str,
    ) {
        let Some(instance) = self.state.layout().pane(pane) else {
            return;
        };
        let mut fields = instance
            .state
            .as_value()
            .as_object()
            .cloned()
            .unwrap_or_default();
        fields.insert(SESSION_KEY.to_owned(), serde_json::Value::from(session));
        let Ok(state) = PaneState::new(serde_json::Value::Object(fields)) else {
            return;
        };
        let pane = pane.clone();
        self.state
            .apply(vec![Command::SetPaneState { pane, state }]);
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::core_panes::TERMINAL;
    use crate::shell_view::test_support::{panes_of, with_terminal};

    #[gpui::test]
    fn a_shell_that_exits_closes_its_tab(cx: &mut TestAppContext) {
        let (mut opened, terminal) = with_terminal(cx);
        let left = opened
            .shell
            .update_in(&mut opened.window, |shell, window, cx| {
                shell.on_terminal((&terminal, terminalview::Event::Exited(0)), window, cx);
                panes_of(shell, TERMINAL)
            });
        assert!(left.is_empty(), "{left:?}");
    }

    #[gpui::test]
    fn a_saved_session_that_ended_unseen_closes_its_tab(cx: &mut TestAppContext) {
        let (mut opened, terminal) = with_terminal(cx);
        let left = opened.shell.update(&mut opened.window, |shell, cx| {
            shell.on_terminal_session((&terminal, Report::Gone), cx);
            panes_of(shell, TERMINAL)
        });
        assert!(left.is_empty(), "{left:?}");
    }

    #[gpui::test]
    fn a_bound_session_is_saved_with_its_pane_for_the_next_start(cx: &mut TestAppContext) {
        let (mut opened, terminal) = with_terminal(cx);
        let saved = opened.shell.update(&mut opened.window, |shell, cx| {
            shell.on_terminal_session((&terminal, Report::Bound("s9".into())), cx);
            let instance = shell.state.layout().pane(&terminal).cloned();
            instance.and_then(|i| i.state.as_value().get(SESSION_KEY).cloned())
        });
        assert_eq!(saved, Some(serde_json::json!("s9")));
    }
}
