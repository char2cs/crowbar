//! The dialog that asks a [`Question`].

use gpui::{AnyElement, Context, IntoElement, ParentElement, Styled, div};
use ui::{Button, ButtonVariant, Dialog, DialogSize, IconName, Surface, Theme};

use crate::sidebar::Sidebar;
use crate::sidebar::{SPACE_MD, TEXT_LARGE};

impl Sidebar {
    pub(crate) fn question_layer(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Option<AnyElement> {
        let question = self.question.clone()?;
        let palette = Theme::of(cx).palette(Surface::Window);
        let sidebar = cx.entity();
        let cancel = Button::new("question-cancel", "Cancel")
            .variant(ButtonVariant::Ghost)
            .on_click(cx.listener(|this, _, _, cx| this.cancel_question(cx)));
        let accept = Button::new("question-accept", question.confirm)
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

#[cfg(test)]
mod tests {
    use super::*;
    use sidebar::{Command, Intent, Question, RowId};

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
    fn the_kill_question_floats_above_the_panes_and_answers_a_click(cx: &mut TestAppContext) {
        let mut floating = crate::test_support::open_floating(cx, DemoShell::reference());
        let sidebar = floating.sidebar.clone();
        let question = Question::kill(RowId::from("chat"), "Refactor");
        sidebar.update_in(&mut floating.window, |this, window, cx| {
            this.apply(Intent::Ask(question), window, cx);
        });
        floating.window.run_until_parked();
        assert_eq!(
            floating.window.debug_bounds("ui-dialog"),
            None,
            "nothing in the window"
        );
        let card = floating.top.debug_bounds("ui-dialog").unwrap_or_default();
        let accept = point(card.right() - px(40.0), card.bottom() - px(24.0));
        floating.top.simulate_click(accept, gpui::Modifiers::none());
        assert_eq!(
            floating.recorder.sent(),
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
