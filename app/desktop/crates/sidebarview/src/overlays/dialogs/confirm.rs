use gpui::{Context, TaskExt as _, Window};
use sidebar::{Confirmation, RemovalTarget};
use ui::{AlertButton, AlertRequest, AlertRole, NativeAlert};

use crate::sidebar::Sidebar;

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
    pub(crate) fn ask_confirmation(
        &self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(confirmation) = self.tree.removals().confirmation() else {
            return;
        };
        let answer = NativeAlert::ask(confirmation_request(confirmation), window, cx);
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
    use sidebar::{Intent, WorkAtRisk};

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
        cx.run_until_parked();
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
        let cases = [
            (
                Some(1),
                vec![Command::RemoveDiscardingWork(chat.clone())],
                false,
            ),
            (None, Vec::new(), true),
        ];
        for (pressed, sent, back) in cases {
            crate::test_support::answer_alerts_with(cx, pressed);
            let (recorder, sidebar, cx) = open(cx, DemoShell::reference());
            asked_to_discard(&sidebar, cx);
            assert_eq!(recorder.sent(), sent, "{pressed:?}: sent");
            let shown = sidebar.read_with(cx, |this, _| shown(this, &chat));
            assert_eq!(shown, back, "{pressed:?}: the row is back only when kept");
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

    #[gpui::test]
    fn without_a_native_dialog_deleting_a_space_is_cancelled_and_nothing_is_drawn(
        cx: &mut TestAppContext
    ) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::rich());
        request_space_removal(&sidebar, cx);
        assert_eq!(cx.debug_bounds("ui-dialog"), None, "nothing is drawn");
        assert_eq!(recorder.sent(), Vec::<Command>::new());
        let pending =
            sidebar.read_with(cx, |this, _| this.tree.removals().confirmation().is_some());
        assert!(!pending, "the question counts as cancelled");
    }
}
