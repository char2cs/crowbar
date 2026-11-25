//! The dialog that asks a [`Question`].

use gpui::{
    AnyElement, Context, IntoElement, ParentElement, Styled, Task, TaskExt as _, Window, div,
};
use sidebar::{Command, Question, Stakes};
use ui::{
    AlertButton, AlertRequest, AlertRole, Button, ButtonVariant, Dialog, DialogSize, IconName,
    NativeAlert, Surface, Theme,
};

use crate::sidebar::Sidebar;
use crate::sidebar::{SPACE_MD, TEXT_LARGE};

pub(crate) const ACCEPTED: Option<usize> = Some(1);

impl Sidebar {
    pub(crate) fn question_layer(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Option<AnyElement> {
        let question = self.question.clone()?;
        let palette = Theme::of(cx).palette(Surface::Window);
        let sidebar = cx.entity();
        let cancel = Button::new("question-cancel", question.cancel)
            .variant(ButtonVariant::Ghost)
            .on_click(cx.listener(|this, _, _, cx| this.cancel_question(cx)));
        let accept = Button::new("question-accept", question.confirm)
            .variant(accept_variant(question.stakes))
            .on_click(cx.listener(|this, _, _, cx| this.accept_question(cx)));
        let body = div()
            .text_size(TEXT_LARGE)
            .text_color(palette.muted_foreground())
            .child(question.body);
        Some(
            Dialog::new("sidebar-question")
                .title(question.title)
                .icon(IconName::Warning)
                .size(DialogSize::Md)
                .body(body)
                .footer(div().flex().gap(SPACE_MD).child(cancel).child(accept))
                .on_close(move |_, cx| sidebar.update(cx, Self::cancel_question))
                .into_any_element(),
        )
    }

    pub(crate) fn ask(
        &mut self,
        question: Question,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(native) = NativeAlert::installed(cx) else {
            self.question = Some(question);
            return;
        };
        let answer = native.ask(question_request(&question), window, cx);
        Self::send_when_accepted((answer, question.command), window, cx);
    }

    pub(crate) fn send_when_accepted(
        asked: (Task<Option<usize>>, Command),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let (answer, command) = asked;
        cx.spawn_in(window, async move |this, cx| {
            let accepted = (answer.await == ACCEPTED).then_some(command);
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

    pub(crate) fn cancel_question(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.question = None;
        cx.notify();
    }

    pub(crate) fn accept_question(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(question) = self.question.take() {
            self.send(question.command, cx);
        }
        cx.notify();
    }
}

fn question_request(question: &Question) -> AlertRequest {
    let accept = match question.stakes {
        Stakes::Disruptive => AlertRole::Accept,
        Stakes::Destructive => AlertRole::Destructive,
    };
    AlertRequest {
        title: Some(question.title.clone().into()),
        message: question.body.clone().into(),
        buttons: vec![
            AlertButton::new(question.cancel, AlertRole::Cancel),
            AlertButton::new(question.confirm, accept),
        ],
    }
}

fn accept_variant(stakes: Stakes) -> ButtonVariant {
    match stakes {
        Stakes::Disruptive => ButtonVariant::Primary,
        Stakes::Destructive => ButtonVariant::Destructive,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use sidebar::{Intent, RowId};

    use crate::test_support::prelude::*;

    fn ask(
        sidebar: &gpui::Entity<Sidebar>,
        cx: &mut gpui::VisualTestContext,
    ) {
        let question = Question::kill(RowId::from("chat"), "Refactor");
        sidebar.update_in(cx, |this, window, cx| {
            this.apply(Intent::Ask(question), window, cx);
        });
    }

    #[gpui::test]
    fn a_question_sends_its_command_only_when_accepted(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::reference());
        ask(&sidebar, cx);
        sidebar.update(cx, Sidebar::cancel_question);
        assert!(recorder.sent().is_empty(), "cancelling sends nothing");
        ask(&sidebar, cx);
        assert!(recorder.sent().is_empty(), "asking sends nothing");
        sidebar.update(cx, Sidebar::accept_question);
        assert_eq!(
            recorder.sent(),
            vec![Command::KillAgent(RowId::from("chat"))]
        );
        assert!(sidebar.read_with(cx, |this, _| this.question.is_none()));
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

    #[gpui::test]
    fn escape_cancels_the_question(cx: &mut TestAppContext) {
        let mut floating = crate::test_support::open_floating(cx, DemoShell::reference());
        let sidebar = floating.sidebar.clone();
        let question = Question::kill(RowId::from("chat"), "Refactor");
        sidebar.update_in(&mut floating.window, |this, window, cx| {
            this.apply(Intent::Ask(question), window, cx);
        });
        floating.window.run_until_parked();
        crate::test_support::press(&mut floating.top, "escape");
        assert_eq!(floating.recorder.sent(), Vec::new());
        assert!(sidebar.read_with(&floating.window, |this, _| this.question.is_none()));
    }
}
