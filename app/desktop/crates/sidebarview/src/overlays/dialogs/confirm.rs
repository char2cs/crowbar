//! The delete confirmation dialog.

use gpui::{
    AnyElement, App, Context, Div, FontWeight, HighlightStyle, IntoElement, ParentElement, Styled,
    StyledText, TaskExt as _, Window, div,
};
use sidebar::{Confirmation, RemovalTarget, WorkAtRisk};
use ui::{
    AlertButton, AlertRequest, AlertRole, Button, ButtonVariant, Dialog, DialogSize, DialogStyle,
    NativeAlert, Palette, Surface, Theme,
};

use crate::sidebar::Sidebar;
use crate::sidebar::{SPACE_MD, SPACE_XL, SPACE_XS, TEXT_LARGE};

fn risk_row(
    work: &WorkAtRisk,
    mono: &'static str,
    palette: &Palette,
) -> Div {
    let branch = div()
        .font_family(mono)
        .text_color(palette.foreground())
        .child(work.branch.clone());
    let summary = div()
        .text_color(palette.muted_foreground())
        .child(work.summary());
    div().flex().flex_col().child(branch).child(summary)
}

fn lead(
    confirmation: &Confirmation,
    palette: &Palette,
) -> StyledText {
    let (strong, rest) = confirmation.lead();
    let bold = HighlightStyle {
        font_weight: Some(FontWeight::BOLD),
        color: Some(palette.foreground()),
        ..HighlightStyle::default()
    };
    StyledText::new(format!("{strong} {rest}")).with_highlights([(0..strong.len(), bold)])
}

fn work_list(
    confirmation: &Confirmation,
    mono: &'static str,
    palette: &Palette,
) -> Div {
    div()
        .flex()
        .flex_col()
        .gap(SPACE_XS)
        .px(SPACE_XL)
        .pb(SPACE_XL)
        .text_size(TEXT_LARGE)
        .line_height(ui::text_size::LINE_HEIGHT)
        .children(
            confirmation
                .at_risk
                .iter()
                .map(|work| risk_row(work, mono, palette)),
        )
}

fn confirmation_request(confirmation: &Confirmation) -> AlertRequest {
    let (strong, rest) = confirmation.lead();
    let risks = confirmation
        .at_risk
        .iter()
        .map(|work| format!("\n\n{}\n{}", work.branch, work.summary()));
    let message = format!("{strong} {rest}{}", risks.collect::<String>());
    let go = AlertButton::new(confirmation.confirm_label(), AlertRole::Destructive);
    AlertRequest::confirm(confirmation.title(), message, "Cancel", go)
}

impl Sidebar {
    pub(crate) fn confirm_layer(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Option<AnyElement> {
        let confirmation = self.drawn_confirmation(cx)?;
        let theme = *Theme::of(cx);
        let palette = theme.palette(Surface::Window);
        let cancel = Button::new("confirm-cancel", "Cancel")
            .variant(ButtonVariant::Ghost)
            .focus_handle(self.confirm_focus.clone())
            .on_click(cx.listener(|this, _, _, cx| this.cancel_confirmation(cx)));
        let delete = Button::new("confirm-delete", confirmation.confirm_label())
            .variant(ButtonVariant::Destructive)
            .on_click(cx.listener(|this, _, _, cx| this.accept_confirmation(cx)));
        let sidebar = cx.entity();
        let list = (!confirmation.at_risk.is_empty())
            .then(|| work_list(confirmation, theme.mono_family(), palette));
        let dialog = Dialog::new("sidebar-confirm")
            .style(DialogStyle::Alert)
            .title(confirmation.title())
            .description(lead(confirmation, palette))
            .initial_focus(self.confirm_focus.clone())
            .size(DialogSize::Md);
        let dialog = match list {
            Some(list) => dialog.body(list),
            None => dialog,
        };
        Some(
            dialog
                .footer(div().flex().gap(SPACE_MD).child(cancel).child(delete))
                .on_close(move |_, cx| sidebar.update(cx, Self::cancel_confirmation))
                .into_any_element(),
        )
    }

    pub(crate) fn drawn_confirmation(
        &self,
        cx: &App,
    ) -> Option<&Confirmation> {
        NativeAlert::installed(cx).map_or_else(|| self.tree.removals().confirmation(), |_| None)
    }

    pub(crate) fn ask_confirmation(
        &self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let (Some(native), Some(confirmation)) = (
            NativeAlert::installed(cx),
            self.tree.removals().confirmation(),
        ) else {
            return;
        };
        let answer = native.ask(confirmation_request(confirmation), window, cx);
        cx.spawn_in(window, async move |this, cx| {
            let pressed = answer.await;
            this.update(cx, |this, cx| this.answer_confirmation(pressed, cx))
        })
        .detach_and_log_err(cx);
    }

    fn answer_confirmation(
        &mut self,
        pressed: Option<usize>,
        cx: &mut Context<'_, Self>,
    ) {
        if pressed == Some(AlertRequest::GO) {
            return self.accept_confirmation(cx);
        }
        self.cancel_confirmation(cx);
    }

    fn cancel_confirmation(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.tree.cancel_removal();
        cx.notify();
    }

    fn accept_confirmation(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(confirmation) = self.tree.accept_removal() else {
            return;
        };
        let command = confirmation.command();
        match confirmation.target {
            RemovalTarget::Row(row) => self.send_removal((&row, command), cx),
            RemovalTarget::Project(_) => self.send(command, cx),
        }
        cx.notify();
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use sidebar::Intent;

    use crate::test_support::prelude::*;

    fn ask(
        sidebar: &gpui::Entity<Sidebar>,
        cx: &mut gpui::VisualTestContext,
    ) {
        sidebar.update(cx, |this, cx| {
            this.tree
                .request_project_removal(ProjectId::from("inv"), "inv");
            cx.notify();
        });
        cx.run_until_parked();
    }

    #[gpui::test]
    fn a_space_question_deletes_the_space_only_when_accepted(cx: &mut TestAppContext) {
        let cases: [(Answer, Vec<Command>); 2] = [
            (Sidebar::cancel_confirmation, Vec::new()),
            (
                Sidebar::accept_confirmation,
                vec![Command::DeleteProject(ProjectId::from("inv"))],
            ),
        ];
        for (answer, sent) in cases {
            let (recorder, sidebar, cx) = open(cx, DemoShell::rich());
            ask(&sidebar, cx);
            sidebar.update(cx, answer);
            let pending =
                sidebar.read_with(cx, |this, _| this.tree.removals().confirmation().is_some());
            assert_eq!((recorder.sent(), pending), (sent, false));
        }
    }

    type Answer = fn(&mut Sidebar, &mut Context<'_, Sidebar>);
    type Case = (&'static str, Answer, Vec<Command>, bool);

    fn asked_to_discard(
        sidebar: &gpui::Entity<Sidebar>,
        cx: &mut gpui::VisualTestContext,
    ) {
        let risk = WorkAtRisk {
            branch: "feature/x".into(),
            uncommitted_files: 0,
            unmerged_commits: 1,
        };
        sidebar.update_in(cx, |this, window, cx| {
            this.apply(Intent::RequestRemoval(RowId::from("chat")), window, cx);
            this.ask_to_discard((&RowId::from("chat"), vec![risk]), window, cx);
        });
    }

    fn shown(
        sidebar: &Sidebar,
        row: &RowId,
    ) -> bool {
        let held = sidebar.tree.removals().tray().get(row).is_some();
        sidebar.row(row).is_some() && !held
    }

    #[gpui::test]
    fn only_accepting_the_second_question_resends_the_removal_with_consent(
        cx: &mut TestAppContext
    ) {
        let chat = RowId::from("chat");
        let cases: [Case; 2] = [
            (
                "accept",
                Sidebar::accept_confirmation,
                vec![Command::RemoveDiscardingWork(chat.clone())],
                false,
            ),
            ("cancel", Sidebar::cancel_confirmation, Vec::new(), true),
        ];
        for (name, answer, sent, back) in cases {
            let (recorder, sidebar, cx) = open(cx, DemoShell::reference());
            asked_to_discard(&sidebar, cx);
            sidebar.update(cx, answer);
            assert_eq!(recorder.sent(), sent, "{name}: sent");
            let shown = sidebar.read_with(cx, |this, _| shown(this, &chat));
            assert_eq!(shown, back, "{name}: the row is back only when kept");
        }
    }

    fn request_space_removal(
        sidebar: &gpui::Entity<Sidebar>,
        cx: &mut gpui::VisualTestContext,
    ) {
        sidebar.update_in(cx, |this, window, cx| {
            let ask = Intent::RequestProjectRemoval(ProjectId::from("inv"));
            this.apply(ask, window, cx);
        });
        cx.run_until_parked();
    }

    #[gpui::test]
    fn with_a_native_dialog_deleting_a_space_is_asked_there_and_only_its_accept_deletes(
        cx: &mut TestAppContext
    ) {
        let delete = vec![Command::DeleteProject(ProjectId::from("inv"))];
        for (pressed, sent) in [(Some(1), delete), (None, Vec::new())] {
            let asked = crate::test_support::answer_alerts_with(cx, pressed);
            let (recorder, sidebar, cx) = open(cx, DemoShell::rich());
            request_space_removal(&sidebar, cx);
            assert_eq!(asked.borrow().len(), 1, "{pressed:?}: asked once");
            assert_eq!(cx.debug_bounds("ui-dialog"), None, "nothing is drawn");
            assert_eq!(recorder.sent(), sent, "{pressed:?}");
            let pending =
                sidebar.read_with(cx, |this, _| this.tree.removals().confirmation().is_some());
            assert!(!pending, "{pressed:?}: the question is answered");
        }
    }
}
