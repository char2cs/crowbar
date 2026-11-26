//! Asks a [`Question`] as the desktop's message dialog.

use gpui::{Context, Task, TaskExt as _, Window};
use sidebar::{Command, Question, Stakes};
use ui::{AlertButton, AlertRequest, AlertRole, NativeAlert};

use crate::sidebar::Sidebar;

impl Sidebar {
    pub(crate) fn ask(
        question: Question,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let answer = NativeAlert::ask(question_request(&question), window, cx);
        Self::send_when_accepted((answer, question.command), window, cx);
    }

    pub(crate) fn send_when_accepted(
        asked: (Task<Option<usize>>, Command),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let (answer, command) = asked;
        cx.spawn_in(window, async move |this, cx| {
            let accepted = (answer.await == Some(AlertRequest::GO)).then_some(command);
            this.update(cx, |this, cx| this.send_answered(accepted, cx))
        })
        .detach_and_log_err(cx);
    }

    fn send_answered(
        &mut self,
        accepted: Option<Command>,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(command) = accepted {
            self.send(command, cx);
        }
    }
}

fn question_request(question: &Question) -> AlertRequest {
    let accept = match question.stakes {
        Stakes::Disruptive => AlertRole::Accept,
        Stakes::Destructive => AlertRole::Destructive,
    };
    let go = AlertButton::new(question.confirm, accept);
    AlertRequest::confirm(
        question.title.clone(),
        question.body.clone(),
        question.cancel,
        go,
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    use sidebar::{Intent, RowId};

    use crate::test_support::prelude::*;

    #[gpui::test]
    fn without_a_native_dialog_the_kill_question_is_cancelled_and_nothing_is_drawn(
        cx: &mut TestAppContext
    ) {
        let mut floating = crate::test_support::open_floating(cx, DemoShell::reference());
        let sidebar = floating.sidebar.clone();
        let question = Question::kill(RowId::from("chat"), "Refactor");
        sidebar.update_in(&mut floating.window, |this, window, cx| {
            this.apply(Intent::Ask(question), window, cx);
        });
        floating.window.run_until_parked();
        assert_eq!(floating.window.debug_bounds("ui-dialog"), None);
        assert!(
            !ui::testing::covered(&mut floating.window).0,
            "the panes stay shown"
        );
        assert_eq!(floating.recorder.sent(), Vec::new());
    }

    #[gpui::test]
    fn with_a_native_dialog_the_kill_question_is_asked_there_and_its_accept_kills(
        cx: &mut TestAppContext
    ) {
        let asked = crate::test_support::answer_alerts_with(cx, Some(1));
        let (recorder, sidebar, cx) = open(cx, DemoShell::reference());
        let question = Question::kill(RowId::from("chat"), "Refactor");
        let (title, cancel, confirm) = (question.title.clone(), question.cancel, question.confirm);
        sidebar.update_in(cx, |this, window, cx| {
            this.apply(Intent::Ask(question), window, cx);
        });
        cx.run_until_parked();
        let request = asked.borrow().first().cloned();
        let words = request.map(|request| {
            let labels: Vec<String> = request
                .buttons
                .iter()
                .map(|b| b.label.to_string())
                .collect();
            (request.title.map(|title| title.to_string()), labels)
        });
        let want = (Some(title), vec![cancel.to_owned(), confirm.to_owned()]);
        assert_eq!(words, Some(want));
        assert_eq!(cx.debug_bounds("ui-dialog"), None, "nothing is drawn");
        assert_eq!(
            recorder.sent(),
            vec![Command::KillAgent(RowId::from("chat"))]
        );
    }
}
