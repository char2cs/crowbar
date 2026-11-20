//! Opening a row: a checkout opens as its chat, a container folds.

use gpui::Context;
use sidebar::{Command, RowId};

use crate::sidebar::Sidebar;

impl Sidebar {
    pub(crate) fn open_row(
        &mut self,
        id: RowId,
        cx: &mut Context<'_, Self>,
    ) {
        if self.row(&id).is_some_and(|row| row.is_container()) {
            self.tree.toggle_fold(&id);
            Self::changed(cx);
            return;
        }
        self.send(Command::OpenRow(id), cx);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::sidebar::row_action::RowAction;
    use crate::test_support::prelude::*;

    fn opened(
        cx: &mut TestAppContext,
        row: &str,
    ) -> (usize, Vec<Command>) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update_in(cx, |this, window, cx| {
            this.act(RowAction::Open(RowId::from(row)), window, cx);
        });
        let folded = sidebar.read_with(cx, |this, _| this.tree.folded().count());
        (folded, recorder.sent())
    }

    #[gpui::test]
    fn a_checkout_opens_as_its_chat_and_a_container_folds(cx: &mut TestAppContext) {
        const CASES: &[(&str, bool)] = &[("chat", false), ("demo", true), ("main", true)];
        for (row, folds) in CASES {
            let (folded, sent) = opened(cx, row);
            assert_eq!(folded == 1, *folds, "{row} folds");
            let opens = sent == vec![Command::OpenRow(RowId::from(*row))];
            assert_eq!(opens, !*folds, "{row} opens");
        }
    }
}
