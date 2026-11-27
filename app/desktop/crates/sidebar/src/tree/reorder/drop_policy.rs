use crate::shell::Activity;
use crate::shell::Lock;
use crate::shell::Row;
use crate::shell::RowKind;
use crate::shell::Worktree;
use crate::tree::reorder::drop_modes::DropModes;
use crate::tree::reorder::row_tree::RowTree;

impl DropModes {
    pub fn allowed(
        rows: &[Row],
        subject: usize,
        target: usize,
    ) -> Self {
        let (Some(dragged), Some(over)) = (rows.get(subject), rows.get(target)) else {
            return DropModes::NONE;
        };
        let blocked = subject == target || is_busy(dragged) || is_busy(over);
        if blocked || is_inside(rows, subject, target) {
            return DropModes::NONE;
        }
        let working = dragged.activity == Activity::Working;
        if working && dragged.kind != RowKind::Chat {
            return DropModes::NONE;
        }
        let tree = RowTree::new(rows);
        match dragged.kind {
            RowKind::Chat if working => leaving(tree, (subject, target)),
            RowKind::Chat => DropModes::ALL,
            RowKind::RepoHeader => repository(rows, target),
            RowKind::Folder if tree.repo(Some(subject)).is_none() => home_folder(rows, target),
            RowKind::Folder | RowKind::Branch => in_repo(rows, (subject, target)),
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

fn leaving(
    tree: RowTree<'_>,
    (subject, target): (usize, usize),
) -> DropModes {
    let place = |from| (tree.repo(from), tree.anchor(from));
    let own = place(Some(subject));
    let beside = (place(tree.parent(target)) != own).then_some(DropModes::REORDER);
    let into = (place(Some(target)) != own).then_some(DropModes::INTO);
    DropModes::union([beside, into])
}

fn home_folder(
    rows: &[Row],
    target: usize,
) -> DropModes {
    if rows
        .get(target)
        .is_some_and(|row| row.kind == RowKind::RepoHeader)
    {
        return DropModes::REORDER;
    }
    if RowTree::new(rows).repo(Some(target)).is_some() {
        return DropModes::NONE;
    }
    DropModes::ALL
}

fn repository(
    rows: &[Row],
    target: usize,
) -> DropModes {
    let Some(over) = rows.get(target) else {
        return DropModes::NONE;
    };
    if over.kind == RowKind::RepoHeader {
        return DropModes::REORDER;
    }
    let tree = RowTree::new(rows);
    if tree.repo(Some(target)).is_some() {
        return DropModes::NONE;
    }
    let holds_repositories = |index: usize| {
        rows.get(index)
            .is_some_and(|row| row.kind == RowKind::Folder)
    };
    let safe = tree.parent(target).is_none_or(holds_repositories);
    let beside = safe.then_some(DropModes::REORDER);
    let into = (over.kind == RowKind::Folder).then_some(DropModes::INTO);
    DropModes::union([beside, into])
}

fn in_repo(
    rows: &[Row],
    (subject, target): (usize, usize),
) -> DropModes {
    let tree = RowTree::new(rows);
    let (Some(dragged), Some(over)) = (rows.get(subject), rows.get(target)) else {
        return DropModes::NONE;
    };
    if tree.repo(Some(subject)) != tree.repo(Some(target)) {
        return DropModes::NONE;
    }
    if dragged.kind == RowKind::Branch && !dragged.is_ownerless() {
        return DropModes::ALL;
    }
    let from = match dragged.kind {
        RowKind::Folder => Some(subject),
        _ => tree.parent(subject),
    };
    let own = tree.anchor(from);
    let beside = (tree.anchor(tree.parent(target)) == own).then_some(DropModes::REORDER);
    let files = tree.anchor(Some(target)) == own;
    let files = files && (dragged.kind == RowKind::Folder || over.kind == RowKind::Folder);
    let into = files.then_some(DropModes::INTO);
    DropModes::union([beside, into])
}

impl Row {
    pub fn is_ownerless(&self) -> bool {
        self.kind == RowKind::Branch
            && (self.lock == Lock::Locked || self.worktree == Worktree::Shared)
    }
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
            "a working chat leaves its workspace beside or into a row",
            working_chat,
            (2, 3),
            DropModes::ALL,
        ),
        (
            "a working chat never reorders within its workspace",
            working_chat,
            (2, 1),
            DropModes::REORDER,
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
            "an owned branch reorders or nests at a sibling folder",
            untouched,
            (1, 3),
            DropModes::ALL,
        ),
        (
            "an owned branch moves under a chat of its repository",
            untouched,
            (1, 5),
            DropModes::ALL,
        ),
        (
            "a plain branch files into a folder of its own fork anchor",
            plain_branch,
            (1, 3),
            DropModes::ALL,
        ),
        (
            "no branch leaves its repository",
            untouched,
            (1, 7),
            DropModes::NONE,
        ),
        (
            "a plain branch reorders inside a folder of its own fork anchor",
            plain_branch,
            (1, 4),
            DropModes::REORDER,
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

    fn space() -> Vec<Row> {
        let mut lock = Row::new("lock", RowKind::Branch, 1, "main");
        lock.lock = Lock::Locked;
        let mut owned = Row::new("b", RowKind::Branch, 1, "feature");
        owned.worktree = Worktree::Owned;
        vec![
            Row::new("hf", RowKind::Folder, 0, "notes"),
            Row::new("hc", RowKind::Chat, 0, "hello"),
            Row::new("r", RowKind::RepoHeader, 0, "demo"),
            lock,
            Row::new("t", RowKind::Chat, 2, "thread"),
            Row::new("f", RowKind::Folder, 1, "src"),
            owned,
            Row::new("bf", RowKind::Folder, 2, "inner"),
            Row::new("c", RowKind::Chat, 1, "loose"),
            Row::new("r2", RowKind::RepoHeader, 0, "other"),
            Row::new("c4", RowKind::Chat, 1, "four"),
            Row::new("hc2", RowKind::Chat, 0, "after the repos"),
        ]
    }

    fn working_thread(rows: &mut [Row]) {
        rows[4].activity = Activity::Working;
    }

    const SPACE: &[Case] = &[
        (
            "a folder reorders or nests past a chat of its anchor",
            untouched,
            (5, 8),
            DropModes::ALL,
        ),
        (
            "a folder never crosses into a branch's context",
            untouched,
            (7, 5),
            DropModes::NONE,
        ),
        (
            "a folder files into its own branch",
            untouched,
            (7, 6),
            DropModes::INTO,
        ),
        (
            "a locked branch files into a folder",
            untouched,
            (3, 5),
            DropModes::ALL,
        ),
        (
            "a locked branch reorders past a chat",
            untouched,
            (3, 8),
            DropModes::REORDER,
        ),
        (
            "a repo files into a home folder",
            untouched,
            (2, 0),
            DropModes::ALL,
        ),
        (
            "a repo reorders past a home chat",
            untouched,
            (2, 1),
            DropModes::REORDER,
        ),
        (
            "a repo never enters another",
            untouched,
            (9, 8),
            DropModes::NONE,
        ),
        (
            "a home folder reorders past a repo",
            untouched,
            (0, 2),
            DropModes::REORDER,
        ),
        (
            "a home row below the repos is not in one",
            untouched,
            (0, 11),
            DropModes::ALL,
        ),
        (
            "a home folder never enters a repo",
            untouched,
            (0, 5),
            DropModes::NONE,
        ),
        (
            "a working thread moves to the checkout",
            working_thread,
            (4, 8),
            DropModes::ALL,
        ),
        (
            "a working thread only reorders past its own branch",
            working_thread,
            (4, 3),
            DropModes::REORDER,
        ),
    ];

    #[test]
    fn a_whole_space_moves_as_it_does_on_develop() {
        for (name, setup, (subject, target), expected) in SPACE {
            let mut rows = space();
            setup(&mut rows);
            assert_eq!(
                DropModes::allowed(&rows, *subject, *target),
                *expected,
                "{name}"
            );
        }
    }
}
