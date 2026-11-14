//! Turning the visible rows of one space into elements.

use std::rc::Rc;
use std::time::Instant;

use gpui::{AnyElement, IntoElement};

use crate::editing::Editing;
use crate::expansion::Expansion;
use crate::operation_row::OperationRow;
use crate::project_tree::ProjectTree;
use crate::removing_row::RemovingRow;
use crate::row::Row;
use crate::row_action::RowHandler;
use crate::row_input::RowInput;
use crate::row_kind::RowKind;
use crate::sidebar::Sidebar;
use crate::tree_row::TreeRow;
use crate::visible_row::VisibleRow;
use crate::worktree::Worktree;

impl Sidebar {
    pub(crate) fn tree_rows(&self, tree: &ProjectTree, handler: &RowHandler) -> Vec<AnyElement> {
        let now = Instant::now();
        self.tree
            .visible(&tree.rows)
            .into_iter()
            .filter_map(|visible| Some((tree.rows.get(visible.index)?.clone(), visible)))
            .flat_map(|(row, visible)| {
                let draft = self.draft_below(&row, handler);
                [Some(self.row_element((row, visible), now, handler)), draft]
            })
            .flatten()
            .collect()
    }

    fn row_element(
        &self,
        (row, visible): (Row, VisibleRow),
        now: Instant,
        handler: &RowHandler,
    ) -> AnyElement {
        let handler = Rc::clone(handler);
        if let Some(hold) = visible.removal {
            return RemovingRow {
                row,
                hold,
                now,
                handler,
            }
            .into_any_element();
        }
        if row.operation.is_some() {
            return OperationRow { row, handler }.into_any_element();
        }
        let input = self.input_for(&row);
        let expansion = visible.expansion;
        TreeRow {
            row,
            expansion,
            input,
            handler,
        }
        .into_any_element()
    }

    fn input_for(&self, row: &Row) -> Option<RowInput> {
        let edit = self.inline.as_ref().filter(|edit| edit.row == row.id)?;
        let wanted = matches!(
            self.tree.editing(),
            Editing::Renaming(_) | Editing::Branching(_)
        );
        wanted.then(|| RowInput {
            field: edit.field.clone(),
            error: edit.error.clone(),
        })
    }

    fn draft_below(&self, parent: &Row, handler: &RowHandler) -> Option<AnyElement> {
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
            field: edit.field.clone(),
            error: edit.error.clone(),
        };
        let handler = Rc::clone(handler);
        let expansion = Expansion::Expanded;
        Some(
            TreeRow {
                row: draft,
                expansion,
                input: Some(input),
                handler,
            }
            .into_any_element(),
        )
    }
}

#[cfg(test)]
#[path = "tree_rows_tests.rs"]
mod tests;
