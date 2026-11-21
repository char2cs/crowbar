//! Which placements the tree allows for a dragged row over a target row.

use crate::shell::Activity;
use crate::shell::Lock;
use crate::shell::Row;
use crate::shell::RowKind;
use crate::shell::Worktree;
use crate::tree::reorder::drop_modes::DropModes;

impl DropModes {
    /// The placements allowed when `subject` (an index into `rows`) is dragged over `target`.
    /// Illegal drops return [`DropModes::NONE`] and show no indicator.
    pub fn allowed(
        rows: &[Row],
        subject: usize,
        target: usize,
    ) -> Self {
        let (Some(dragged), Some(over)) = (rows.get(subject), rows.get(target)) else {
            return DropModes::NONE;
        };
        let blocked = subject == target || is_busy(dragged) || is_busy(over);
        if blocked {
            return DropModes::NONE;
        }
        if is_inside(rows, subject, target) {
            return DropModes::NONE;
        }
        if dragged.activity == Activity::Working {
            return working(dragged, over);
        }
        match dragged.kind {
            RowKind::Chat => DropModes::ALL,
            RowKind::RepoHeader => repo(over),
            RowKind::Folder | RowKind::Branch => container(rows, (subject, target)),
        }
    }
}

fn is_busy(row: &Row) -> bool {
    row.operation.is_some()
}

fn is_inside(
    rows: &[Row],
    ancestor: usize,
    index: usize,
) -> bool {
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

fn working(
    dragged: &Row,
    over: &Row,
) -> DropModes {
    if dragged.kind != RowKind::Chat || over.kind == RowKind::Chat {
        return DropModes::NONE;
    }
    DropModes::INTO
}

fn repo(over: &Row) -> DropModes {
    if over.kind == RowKind::RepoHeader {
        return DropModes::REORDER;
    }
    DropModes::NONE
}

fn container(
    rows: &[Row],
    (subject, target): (usize, usize),
) -> DropModes {
    let (Some(dragged), Some(over)) = (rows.get(subject), rows.get(target)) else {
        return DropModes::NONE;
    };
    if repo_of(rows, subject) != repo_of(rows, target) {
        return DropModes::NONE;
    }
    let ownerless = dragged.is_ownerless();
    let siblings = parent_of(rows, subject) == parent_of(rows, target);
    match (siblings, over.kind, ownerless) {
        (true, _, _) => DropModes::REORDER,
        (false, RowKind::Folder, false) => DropModes::INTO,
        _ => DropModes::NONE,
    }
}

impl Row {
    /// Whether the row is a branch Crowbar does not own: locked, or on a shared worktree.
    /// Such a branch can be reordered among its siblings but never nested into a folder.
    pub fn is_ownerless(&self) -> bool {
        self.kind == RowKind::Branch
            && (self.lock == Lock::Locked || self.worktree == Worktree::Shared)
    }
}

fn parent_of(
    rows: &[Row],
    index: usize,
) -> Option<usize> {
    let depth = rows.get(index)?.depth;
    rows.iter().take(index).rposition(|row| row.depth < depth)
}

fn repo_of(
    rows: &[Row],
    index: usize,
) -> Option<usize> {
    rows.iter()
        .take(index + 1)
        .rposition(|row| row.kind == RowKind::RepoHeader)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::shell::Operation;
    use crate::shell::OperationPhase;

    fn tree() -> Vec<Row> {
        let mut branch = Row::new("b1", RowKind::Branch, 1, "feature");
        branch.worktree = Worktree::Owned;
        vec![
            Row::new("r", RowKind::RepoHeader, 0, "demo"),
            branch,
            Row::new("c1", RowKind::Chat, 2, "one"),
            Row::new("f", RowKind::Folder, 1, "src"),
            Row::new("c2", RowKind::Chat, 2, "two"),
            Row::new("c3", RowKind::Chat, 1, "three"),
            Row::new("r2", RowKind::RepoHeader, 0, "other"),
            Row::new("f2", RowKind::Folder, 1, "docs"),
        ]
    }

    type Setup = fn(&mut [Row]);

    fn untouched(_: &mut [Row]) {}

    fn busy_folder(rows: &mut [Row]) {
        rows[3].operation = Some(Operation {
            label: "New workspace".into(),
            phase: OperationPhase::Running,
        });
    }

    fn working_chat(rows: &mut [Row]) {
        rows[2].activity = Activity::Working;
    }

    fn working_folder(rows: &mut [Row]) {
        rows[3].activity = Activity::Working;
    }

    fn plain_branch(rows: &mut [Row]) {
        rows[1].worktree = Worktree::Shared;
    }

    type Case = (&'static str, Setup, (usize, usize), DropModes);

    const CASES: &[Case] = &[
        ("chat over folder", untouched, (2, 3), DropModes::ALL),
        ("chat over repo", untouched, (2, 0), DropModes::ALL),
        (
            "a chat over a chat reorders or nests as its thread",
            untouched,
            (2, 5),
            DropModes::ALL,
        ),
        ("self", untouched, (3, 3), DropModes::NONE),
        ("own child", untouched, (3, 4), DropModes::NONE),
        (
            "repo into its own folder",
            untouched,
            (0, 3),
            DropModes::NONE,
        ),
        ("target busy", busy_folder, (2, 3), DropModes::NONE),
        ("subject busy", busy_folder, (3, 2), DropModes::NONE),
        (
            "a working chat waits for the turn",
            working_chat,
            (2, 3),
            DropModes::INTO,
        ),
        (
            "no reorder while working",
            working_chat,
            (2, 5),
            DropModes::NONE,
        ),
        (
            "a working folder never moves",
            working_folder,
            (3, 1),
            DropModes::NONE,
        ),
        ("repo over repo", untouched, (0, 6), DropModes::REORDER),
        ("repo over chat", untouched, (0, 5), DropModes::NONE),
        (
            "folder siblings reorder",
            untouched,
            (3, 1),
            DropModes::REORDER,
        ),
        (
            "folder into another repository",
            untouched,
            (3, 7),
            DropModes::NONE,
        ),
        (
            "no nesting for a plain branch",
            plain_branch,
            (1, 4),
            DropModes::NONE,
        ),
    ];

    #[test]
    fn drops_follow_the_placement_rules() {
        for (name, setup, (subject, target), expected) in CASES {
            let mut rows = tree();
            setup(&mut rows);
            assert_eq!(
                DropModes::allowed(&rows, *subject, *target),
                *expected,
                "{name}"
            );
        }
    }
}
