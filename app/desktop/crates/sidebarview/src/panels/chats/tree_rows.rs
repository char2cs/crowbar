//! Turning the visible rows of one space into elements.

use std::rc::Rc;
use std::time::Instant;

use gpui::{AnyElement, IntoElement, WeakEntity};
use sidebar::{
    Command, Editing, Expansion, Hold, ProjectTree, Row, RowKind, SpaceIcon, VisibleRow, Worktree,
};

use crate::panels::chats::kinds::OperationRow;
use crate::panels::chats::kinds::RemovingRow;
use crate::panels::chats::kinds::TreeRow;
use crate::panels::chats::parts::RowInput;
use crate::panels::chats::parts::Trailing;
use crate::panels::chats::reorder::RowDrag;
use crate::sidebar::RowHandler;
use crate::sidebar::Sidebar;
use crate::sidebar::{INDENT_STEP, ROW_MARGIN_X};

fn removing_element(
    (row, hold): (Row, Hold),
    now: Instant,
    handler: RowHandler,
) -> AnyElement {
    RemovingRow {
        row,
        hold,
        now,
        handler,
    }
    .into_any_element()
}

impl Sidebar {
    pub(crate) fn tree_rows(
        &self,
        tree: &ProjectTree,
        (handler, width, me): (&RowHandler, f32, &WeakEntity<Self>),
    ) -> Vec<AnyElement> {
        let now = self.clock.now();
        self.tree
            .visible(&tree.rows)
            .into_iter()
            .filter_map(|visible| Some((tree.rows.get(visible.index)?.clone(), visible)))
            .flat_map(|(row, visible)| {
                let draft = self.draft_below(&row, handler);
                [
                    Some(self.row_element((row, visible), now, (handler, width, me))),
                    draft,
                ]
            })
            .flatten()
            .collect()
    }

    fn row_element(
        &self,
        entry: (Row, VisibleRow),
        now: Instant,
        layout: (&RowHandler, f32, &WeakEntity<Self>),
    ) -> AnyElement {
        let ((mut row, visible), (handler, width, me)) = (entry, layout);
        row.presence = self.open_chats.presence_of(&row.id);
        let handler = Rc::clone(handler);
        if let Some(hold) = visible.removal {
            return removing_element((row, hold), now, handler);
        }
        if row.operation.is_some() {
            return OperationRow { row, handler }.into_any_element();
        }
        let input = self.input_for(&row);
        let expansion = visible.expansion;
        let drag = RowDrag {
            lifted: self.drag.is_lifted(&row.id),
            mode: self.drag.mode_on(&row.id),
            ghost: None,
        };
        let width = width - 2.0 * ROW_MARGIN_X - f32::from(row.depth) * INDENT_STEP;
        let icon_editing = self.edits_icon_of(&row).then(|| me.clone());
        TreeRow {
            row,
            expansion,
            input,
            handler,
            drag,
            bounds: self.row_bounds.clone(),
            width,
            icon_editing,
        }
        .into_any_element()
    }

    fn edits_icon_of(
        &self,
        row: &Row,
    ) -> bool {
        let command = Command::SetRepoIcon {
            row: row.id.clone(),
            icon: SpaceIcon::Default,
        };
        row.kind == RowKind::RepoHeader && self.data.serves(&command)
    }

    fn input_for(
        &self,
        row: &Row,
    ) -> Option<RowInput> {
        let edit = self.inline.as_ref().filter(|edit| edit.row == row.id)?;
        let trailing = match self.tree.editing() {
            Editing::Renaming(_) => Trailing::Kept,
            Editing::Branching(_) => Trailing::Hidden,
            _ => return None,
        };
        Some(RowInput {
            field: edit.input.clone(),
            error: edit.error.clone(),
            trailing,
        })
    }

    fn draft_below(
        &self,
        parent: &Row,
        handler: &RowHandler,
    ) -> Option<AnyElement> {
        let Editing::Drafting(owner) = self.tree.editing() else {
            return None;
        };
        let edit = self.inline.as_ref().filter(|edit| &edit.row == owner)?;
        if owner != &parent.id {
            return None;
        }
        let id = format!("draft:{}", parent.id.as_str());
        let mut draft = Row::new(id, RowKind::Branch, parent.depth + 1, "");
        draft.worktree = Worktree::Owned;
        let input = RowInput {
            field: edit.input.clone(),
            error: edit.error.clone(),
            trailing: Trailing::Hidden,
        };
        let handler = Rc::clone(handler);
        let expansion = Expansion::Expanded;
        Some(
            TreeRow {
                row: draft,
                expansion,
                input: Some(input),
                handler,
                drag: RowDrag::default(),
                bounds: self.row_bounds.clone(),
                width: 0.0,
                icon_editing: None,
            }
            .into_any_element(),
        )
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::prelude::*;

    fn drawn(
        sidebar: &gpui::Entity<Sidebar>,
        cx: &mut gpui::VisualTestContext,
    ) -> (usize, usize) {
        sidebar.update_in(cx, |this, _, cx| {
            let project = this.data.active_project().expect("an active space");
            let tree = this.data.tree(&project);
            let handler = Sidebar::row_handler(cx);
            let me = cx.weak_entity();
            (
                this.tree_rows(&tree, (&handler, 294.0, &me)).len(),
                tree.rows.len(),
            )
        })
    }

    #[gpui::test]
    fn every_visible_row_is_drawn(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::rich());
        let (shown, listed) = drawn(&sidebar, cx);
        assert!(shown > 0 && shown <= listed + 1);
    }

    #[gpui::test]
    fn folding_a_parent_draws_fewer_rows(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::rich());
        let (before, _) = drawn(&sidebar, cx);
        sidebar.update(cx, |this, _| this.tree.toggle_fold(&RowId::from("demo")));
        assert!(drawn(&sidebar, cx).0 < before);
    }
}
