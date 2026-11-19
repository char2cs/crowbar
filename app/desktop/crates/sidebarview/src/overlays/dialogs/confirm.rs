//! The delete confirmation dialog.

use gpui::{AnyElement, Context, Div, FontWeight, IntoElement, ParentElement, Styled, div};
use sidebar::{Confirmation, RemovalTarget, WorkAtRisk};
use ui::{Button, ButtonVariant, Dialog, DialogSize, Palette, Surface, Theme};

use crate::sidebar::Sidebar;
use crate::sidebar::{SPACE_LG, SPACE_MD, TEXT_LARGE};

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

fn body(
    confirmation: &Confirmation,
    mono: &'static str,
    palette: &Palette,
) -> Div {
    let (strong, rest) = confirmation.lead();
    let lead = div()
        .child(
            div()
                .text_color(palette.foreground())
                .font_weight(FontWeight::BOLD)
                .child(strong),
        )
        .child(rest);
    div()
        .flex()
        .flex_col()
        .gap(SPACE_LG)
        .text_size(TEXT_LARGE)
        .text_color(palette.muted_foreground())
        .child(lead)
        .children(
            confirmation
                .at_risk
                .iter()
                .map(|work| risk_row(work, mono, palette)),
        )
}

impl Sidebar {
    pub(crate) fn confirm_layer(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Option<AnyElement> {
        let confirmation = self.tree.removals().confirmation()?;
        let theme = *Theme::of(cx);
        let palette = theme.palette(Surface::Window);
        let cancel = Button::new("confirm-cancel", "Cancel")
            .variant(ButtonVariant::Ghost)
            .on_click(cx.listener(|this, _, _, cx| this.cancel_confirmation(cx)));
        let delete = Button::new("confirm-delete", confirmation.confirm_label())
            .variant(ButtonVariant::Destructive)
            .on_click(cx.listener(|this, _, _, cx| this.accept_confirmation(cx)));
        let sidebar = cx.entity();
        Some(
            Dialog::new("sidebar-confirm")
                .title(confirmation.title())
                .size(DialogSize::Md)
                .body(body(confirmation, theme.mono_family(), palette))
                .footer(div().flex().gap(SPACE_MD).child(cancel).child(delete))
                .on_close(move |_, cx| sidebar.update(cx, Self::cancel_confirmation))
                .into_any_element(),
        )
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
    fn cancelling_drops_the_question_and_sends_nothing(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::rich());
        ask(&sidebar, cx);
        sidebar.update(cx, Sidebar::cancel_confirmation);
        assert!(sidebar.read_with(cx, |this, _| this.tree.removals().confirmation().is_none()));
        assert_eq!(recorder.sent(), Vec::<Command>::new());
    }

    #[gpui::test]
    fn accepting_deletes_the_space(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::rich());
        ask(&sidebar, cx);
        sidebar.update(cx, Sidebar::accept_confirmation);
        assert_eq!(
            recorder.sent(),
            vec![Command::DeleteProject(ProjectId::from("inv"))]
        );
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
            this.ask_to_discard(&RowId::from("chat"), vec![risk], cx);
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

    #[gpui::test]
    fn the_layer_exists_only_while_a_question_is_pending(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::rich());
        assert!(!sidebar.read_with(cx, |this, _| this.tree.removals().confirmation().is_some()));
        ask(&sidebar, cx);
        assert!(sidebar.read_with(cx, |this, _| this.tree.removals().confirmation().is_some()));
    }
}
