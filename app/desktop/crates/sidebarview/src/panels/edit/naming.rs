//! Opening a row's name for editing, its text selected so typing replaces it.

use gpui::{Context, Window};
use sidebar::{Editing, RowId};
use ui::TextInput;

use crate::sidebar::Sidebar;

impl Sidebar {
    pub(crate) fn start_rename(
        &mut self,
        id: &RowId,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(row) = self.row(id) else {
            return;
        };
        self.tree.start_rename(&row);
        if self.tree.editing() != &Editing::Renaming(id.clone()) {
            return;
        }
        self.open_field((id.clone(), &row.label, ""), window, cx);
        self.select_field(cx);
    }

    /// Opens `name` for editing on `row`, a row just made to be named; it may not be drawn yet.
    pub fn name_new_row(
        &mut self,
        (row, name): (RowId, &str),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.tree.start_naming(&row);
        self.open_field((row, name, ""), window, cx);
        self.select_field(cx);
    }

    fn select_field(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(edit) = self.inline.as_ref() {
            edit.input.update(cx, TextInput::select_all);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::panels::edit::outcome::Outcome;
    use crate::test_support::prelude::*;

    fn start(
        sidebar: &gpui::Entity<Sidebar>,
        cx: &mut gpui::VisualTestContext,
    ) {
        sidebar.update_in(cx, |this, window, cx| {
            this.start_rename(&RowId::from("chat"), window, cx);
        });
    }

    #[gpui::test]
    fn a_row_rename_opens_with_the_label_selected(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::reference());
        start(&sidebar, cx);
        cx.run_until_parked();
        cx.simulate_input("Typed");
        sidebar.update(cx, |this, cx| this.finish_edit(Outcome::Confirm, cx));
        let expected = Command::Rename {
            row: RowId::from("chat"),
            name: "Typed".into(),
        };
        assert_eq!(recorder.sent(), vec![expected]);
    }

    #[gpui::test]
    fn naming_a_new_row_opens_its_name_selected(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update_in(cx, |this, window, cx| {
            this.name_new_row((RowId::from("chat"), "New folder"), window, cx);
        });
        cx.run_until_parked();
        cx.simulate_input("Notes");
        sidebar.update(cx, |this, cx| this.finish_edit(Outcome::Confirm, cx));
        let expected = Command::Rename {
            row: RowId::from("chat"),
            name: "Notes".into(),
        };
        assert_eq!(recorder.sent(), vec![expected]);
    }
}
