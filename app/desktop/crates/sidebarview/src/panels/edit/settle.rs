//! What the daemon answered for a name typed inline: taken closes the input, refused keeps it
//! open over the reason, as develop's `branch-out-input.tsx` does.

use gpui::Context;
use sidebar::RowId;

use crate::sidebar::Sidebar;

impl Sidebar {
    /// The daemon answered the name typed into `row`'s input: taken closes it, a `refusal`
    /// keeps it open with the reason under it.
    pub fn name_answered(
        &mut self,
        row: &RowId,
        refusal: Option<String>,
        cx: &mut Context<'_, Self>,
    ) {
        if !self.names(row) {
            return;
        }
        match refusal {
            Some(reason) => self.refuse_edit(reason, cx),
            None => self.close_edit(cx),
        }
    }

    fn names(
        &self,
        row: &RowId,
    ) -> bool {
        self.inline.as_ref().is_some_and(|edit| &edit.row == row)
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::panels::edit::outcome::Outcome;
    use crate::test_support::prelude::*;

    /// What the open input shows: `None` once it closed, else its error text ("" for none).
    fn shown(
        sidebar: &gpui::Entity<Sidebar>,
        cx: &mut gpui::VisualTestContext,
    ) -> Option<String> {
        sidebar.read_with(cx, |this, _| {
            let edit = this.inline.as_ref()?;
            Some(edit.error.clone().unwrap_or_default())
        })
    }

    #[gpui::test]
    fn a_sent_branch_name_waits_for_the_daemon_and_a_refusal_shows_under_the_input(
        cx: &mut TestAppContext
    ) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        let row = RowId::from("chat");
        sidebar.update_in(cx, |this, window, cx| {
            this.start_branching(&row, window, cx);
        });
        let field = sidebar.read_with(cx, |this, _| {
            this.inline.as_ref().map(|edit| edit.input.clone())
        });
        let field = field.expect("an input is open");
        field.update(cx, |field, cx| field.set_text("bad..name", cx));
        sidebar.update(cx, |this, cx| this.finish_edit(Outcome::Confirm, cx));
        assert_eq!(
            shown(&sidebar, cx).as_deref(),
            Some(""),
            "open while the daemon answers"
        );
        let reason = "\"bad..name\" is not a valid branch name".to_owned();
        sidebar.update(cx, |this, cx| {
            this.name_answered(&row, Some(reason.clone()), cx);
        });
        assert_eq!(
            shown(&sidebar, cx),
            Some(reason),
            "still open, over the reason"
        );
        sidebar.update(cx, |this, cx| this.name_answered(&row, None, cx));
        assert_eq!(shown(&sidebar, cx), None, "closed once taken");
    }
}
