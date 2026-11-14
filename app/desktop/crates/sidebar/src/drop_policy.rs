//! Which placements the tree allows for a dragged row over a target row.

use crate::activity::Activity;
use crate::drop_modes::DropModes;
use crate::lock::Lock;
use crate::row::Row;
use crate::row_kind::RowKind;
use crate::worktree::Worktree;

/// The placements allowed when `subject` (an index into `rows`) is dragged over `target`.
/// Illegal drops return [`DropModes::NONE`] and show no indicator.
pub fn allowed_modes(rows: &[Row], subject: usize, target: usize) -> DropModes {
    let (Some(dragged), Some(over)) = (rows.get(subject), rows.get(target)) else {
        return DropModes::NONE;
    };
    if subject == target || is_busy(dragged) || is_busy(over) {
        return DropModes::NONE;
    }
    if is_inside(rows, subject, target) {
        return DropModes::NONE;
    }
    if dragged.activity == Activity::Working {
        return working(dragged, over);
    }
    match dragged.kind {
        RowKind::Chat => chat(over),
        RowKind::RepoHeader => repo(over),
        RowKind::Folder | RowKind::Branch => container(rows, (subject, target)),
    }
}

fn is_busy(row: &Row) -> bool {
    row.operation.is_some()
}

fn is_inside(rows: &[Row], ancestor: usize, index: usize) -> bool {
    let Some(root) = rows.get(ancestor) else {
        return false;
    };
    index > ancestor
        && rows
            .iter()
            .skip(ancestor + 1)
            .take(index - ancestor)
            .all(|row| row.depth > root.depth)
}

fn working(dragged: &Row, over: &Row) -> DropModes {
    if dragged.kind != RowKind::Chat || over.kind == RowKind::Chat {
        return DropModes::NONE;
    }
    DropModes::INTO
}

fn chat(over: &Row) -> DropModes {
    match over.kind {
        RowKind::Chat => DropModes::REORDER,
        RowKind::RepoHeader | RowKind::Folder | RowKind::Branch => DropModes::ALL,
    }
}

fn repo(over: &Row) -> DropModes {
    if over.kind == RowKind::RepoHeader {
        return DropModes::REORDER;
    }
    DropModes::NONE
}

fn container(rows: &[Row], (subject, target): (usize, usize)) -> DropModes {
    let (Some(dragged), Some(over)) = (rows.get(subject), rows.get(target)) else {
        return DropModes::NONE;
    };
    if repo_of(rows, subject) != repo_of(rows, target) {
        return DropModes::NONE;
    }
    let ownerless = dragged.kind == RowKind::Branch
        && (dragged.lock == Lock::Locked || dragged.worktree == Worktree::Shared);
    let siblings = parent_of(rows, subject) == parent_of(rows, target);
    match (siblings, over.kind, ownerless) {
        (true, _, _) => DropModes::REORDER,
        (false, RowKind::Folder, false) => DropModes::INTO,
        _ => DropModes::NONE,
    }
}

fn parent_of(rows: &[Row], index: usize) -> Option<usize> {
    let depth = rows.get(index)?.depth;
    rows.iter().take(index).rposition(|row| row.depth < depth)
}

fn repo_of(rows: &[Row], index: usize) -> Option<usize> {
    rows.iter()
        .take(index + 1)
        .rposition(|row| row.kind == RowKind::RepoHeader)
}

#[cfg(test)]
#[path = "drop_policy_tests.rs"]
mod tests;
