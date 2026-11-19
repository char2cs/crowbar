//! Sending a held row's delete and putting the row back when it is refused.

use gpui::Context;
use sidebar::{Command, Error, Hold, RowId};
use ui::{ToastContent, ToastKind, ToastLifetime};

use crate::sidebar::Sidebar;

fn reason_of(error: Error) -> String {
    match error {
        Error::Refused { reason } => reason,
        other => other.to_string(),
    }
}

fn refusal_toast(
    hold: Option<&Hold>,
    reason: String,
) -> ToastContent {
    let content = ToastContent::new(ToastKind::Error, reason);
    let Some(hold) = hold else {
        return content;
    };
    content.titled(hold.refusal_title())
}

impl Sidebar {
    /// Shows `row` again after its delete was refused, under a toast titled by the row that
    /// gives `reason`.
    pub fn removal_refused(
        &mut self,
        row: &RowId,
        reason: String,
        cx: &mut Context<'_, Self>,
    ) {
        let toast = refusal_toast(self.tree.removals().tray().get(row), reason);
        self.tree.keep(row);
        self.toast(toast, (None, ToastLifetime::Auto), cx);
        cx.notify();
    }

    pub(crate) fn send_removal(
        &mut self,
        (row, command): (&RowId, Command),
        cx: &mut Context<'_, Self>,
    ) {
        let Err(error) = self.sink.dispatch(command) else {
            cx.notify();
            return;
        };
        self.removal_refused(row, reason_of(error), cx);
    }
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;
    use std::time::Instant;

    use sidebar::{Intent, RemovalTray};

    use super::*;
    use crate::test_support::prelude::*;

    const LOADING: &str = "its chat is still loading — try again in a moment";

    fn shown_toasts(
        sidebar: &gpui::Entity<Sidebar>,
        cx: &mut gpui::VisualTestContext,
    ) -> usize {
        let queue = sidebar.read_with(cx, |this, _| this.toasts.clone());
        queue.read_with(cx, |queue, _| queue.len())
    }

    fn held(
        sidebar: &gpui::Entity<Sidebar>,
        cx: &mut gpui::VisualTestContext,
    ) -> bool {
        let chat = RowId::from("chat");
        sidebar.read_with(cx, |this, _| {
            this.tree.removals().tray().get(&chat).is_some()
        })
    }

    #[test]
    fn the_toast_names_the_row_it_could_not_remove() {
        let mut tree = sidebar::Tree::default();
        let row = sidebar::Row::new("w", sidebar::RowKind::Branch, 1, "feature/one");
        tree.request_removal(&row, Instant::now());
        let toast = refusal_toast(tree.removals().tray().get(&row.id), "busy".into());
        let titled =
            ToastContent::new(ToastKind::Error, "busy").titled("Couldn't remove feature/one");
        assert_eq!(toast, titled, "titled by the row");
        let plain = refusal_toast(None, "busy".into());
        assert_eq!(
            plain,
            ToastContent::new(ToastKind::Error, "busy"),
            "no row, no title"
        );
    }

    #[gpui::test]
    fn a_refused_delete_shows_the_row_again_and_says_why(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update_in(cx, |this, window, cx| {
            this.apply(Intent::RequestRemoval(RowId::from("chat")), window, cx);
        });
        assert!(held(&sidebar, cx), "held while it counts down");
        sidebar.update(cx, |this, cx| {
            this.removal_refused(&RowId::from("chat"), "worktree is busy".into(), cx);
        });
        assert!(!held(&sidebar, cx), "the row is back");
        assert_eq!(shown_toasts(&sidebar, cx), 1, "one toast says why");
    }

    #[gpui::test]
    fn a_delete_the_data_refuses_to_send_shows_the_row_again(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::reference());
        let reading = Rc::new(Cell::new(Instant::now()));
        let seen = Rc::clone(&reading);
        sidebar.update_in(cx, |this, window, cx| {
            this.clock = Rc::new(move || seen.get());
            this.apply(Intent::RequestRemoval(RowId::from("chat")), window, cx);
        });
        let commit = Command::CommitRemoval(RowId::from("chat"));
        recorder.refuse(commit.clone(), LOADING);
        reading.set(reading.get() + RemovalTray::DRAIN);
        cx.executor().advance_clock(RemovalTray::DRAIN);
        cx.run_until_parked();
        assert_eq!(recorder.sent(), vec![commit], "the delete was sent once");
        assert!(!held(&sidebar, cx), "the row is back");
        assert_eq!(shown_toasts(&sidebar, cx), 1, "one toast says why");
    }
}
