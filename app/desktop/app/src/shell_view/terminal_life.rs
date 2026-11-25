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
const NOT_STOPPED: &str = "Stop the closed terminal";

impl ShellView {
    pub(super) fn on_terminal_session(
        &mut self,
        (pane, report): (&PaneId, Report),
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(content) = notice_of(&report) {
            self.show_toast(NoticeKey::Terminal, content, cx);
        }
        match report {
            Report::Bound(session) => self.remember_session(pane, &session),
            Report::Gone => self.close_terminal(pane, cx),
            Report::Failed(reason) => warn_not_started(&reason),
            Report::StopFailed(words) => warn_kept_running(&words),
        }
    }

    pub(super) fn close_terminal(
        &mut self,
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) {
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

fn warn_not_started(reason: &str) {
    tracing::warn!(%reason, "a terminal could not start");
}

fn warn_kept_running(words: &str) {
    tracing::warn!(%words, "a closed terminal kept running");
}

fn notice_of(report: &Report) -> Option<ToastContent> {
    let notice = match report {
        Report::Failed(_) => ToastContent::new(ToastKind::Error, TRY_AGAIN).titled(UNAVAILABLE),
        Report::StopFailed(words) => {
            ToastContent::new(ToastKind::Error, words.clone()).titled(NOT_STOPPED)
        },
        Report::Bound(_) | Report::Gone => return None,
    };
    Some(notice)
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

    #[test]
    fn a_terminal_that_cannot_start_or_stop_is_told_in_develop_s_words() {
        let error = |text: &str, title: &str| {
            Some(ToastContent::new(ToastKind::Error, text.to_owned()).titled(title.to_owned()))
        };
        let cases = [
            (Report::Failed("500".into()), error(TRY_AGAIN, UNAVAILABLE)),
            (
                Report::StopFailed("injected".into()),
                error("injected", NOT_STOPPED),
            ),
            (Report::Bound("s1".into()), None),
            (Report::Gone, None),
        ];
        for (report, notice) in cases {
            assert_eq!(notice_of(&report), notice, "{report:?}");
        }
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
