//! Where a dragged row lands, in the daemon's terms: a parent and an insert index.

use sidebar::{DropMode, Row, RowKind};

use crate::slot::Slot;

pub(crate) fn slot_for(
    rows: &[Row],
    (moved, target): (&str, &str),
    mode: DropMode,
) -> Option<Slot> {
    let lifted = without_subtree(rows, moved)?;
    let at = lifted.iter().position(|row| row.id.as_str() == target)?;
    let target_row = lifted.get(at)?;
    if mode == DropMode::Into {
        return Some(into(&lifted, at, target_row));
    }
    let parent_at = parent_index(&lifted, at);
    let parent = parent_at
        .and_then(|index| lifted.get(index))
        .map_or_default(container_id);
    let siblings: Vec<&str> = match parent_at {
        Some(index) => children_of(&lifted, index)
            .map(|row| row.id.as_str())
            .collect(),
        None => roots(&lifted).map(|row| row.id.as_str()).collect(),
    };
    let position = siblings.iter().position(|id| *id == target)?;
    let index = position + usize::from(mode == DropMode::After);
    Some(Slot { parent, index })
}

fn into(
    rows: &[Row],
    at: usize,
    target: &Row,
) -> Slot {
    Slot {
        parent: container_id(target),
        index: children_of(rows, at).count(),
    }
}

fn container_id(row: &Row) -> String {
    if row.kind == RowKind::RepoHeader {
        return String::new();
    }
    row.id.as_str().to_owned()
}

fn without_subtree(
    rows: &[Row],
    moved: &str,
) -> Option<Vec<Row>> {
    let at = rows.iter().position(|row| row.id.as_str() == moved)?;
    let depth = rows.get(at)?.depth;
    let end = rows
        .iter()
        .skip(at + 1)
        .position(|row| row.depth <= depth)
        .map_or(rows.len(), |n| at + 1 + n);
    Some(
        rows.iter()
            .enumerate()
            .filter(|(i, _)| *i < at || *i >= end)
            .map(|(_, row)| row.clone())
            .collect(),
    )
}

fn parent_index(
    rows: &[Row],
    at: usize,
) -> Option<usize> {
    let depth = rows.get(at)?.depth;
    (0..at)
        .rev()
        .find(|i| rows.get(*i).is_some_and(|row| row.depth < depth))
}

fn children_of(
    rows: &[Row],
    parent: usize,
) -> impl Iterator<Item = &Row> {
    let depth = rows.get(parent).map_or(0, |row| row.depth);
    let below = rows
        .iter()
        .skip(parent + 1)
        .take_while(move |row| row.depth > depth);
    below.filter(move |row| row.depth == depth + 1)
}

fn roots(rows: &[Row]) -> impl Iterator<Item = &Row> {
    rows.iter()
        .filter(|row| row.depth == 0 && row.kind != RowKind::RepoHeader)
}

#[cfg(test)]
mod tests {
    use sidebar::{Row, RowKind};

    use super::*;
    use crate::slot::Slot;

    fn row(
        id: &str,
        kind: RowKind,
        depth: u8,
    ) -> Row {
        Row::new(id, kind, depth, id)
    }

    fn rows() -> Vec<Row> {
        vec![
            row("a", RowKind::Chat, 0),
            row("f", RowKind::Folder, 0),
            row("g", RowKind::Chat, 1),
            row("h", RowKind::Chat, 0),
        ]
    }

    #[test]
    fn before_and_after_count_among_the_siblings_with_the_moved_row_lifted_out() {
        let slot = |moved, target, mode| slot_for(&rows(), (moved, target), mode);
        assert_eq!(
            slot("a", "h", DropMode::Before),
            Some(Slot {
                parent: String::new(),
                index: 1
            })
        );
        assert_eq!(
            slot("h", "a", DropMode::After),
            Some(Slot {
                parent: String::new(),
                index: 1
            })
        );
        assert_eq!(
            slot("a", "g", DropMode::After),
            Some(Slot {
                parent: "f".into(),
                index: 1
            })
        );
    }

    #[test]
    fn into_appends_to_the_container_and_a_row_cannot_go_into_itself() {
        assert_eq!(
            slot_for(&rows(), ("a", "f"), DropMode::Into),
            Some(Slot {
                parent: "f".into(),
                index: 1
            })
        );
        assert_eq!(slot_for(&rows(), ("f", "g"), DropMode::Into), None);
        assert_eq!(slot_for(&rows(), ("a", "missing"), DropMode::Before), None);
    }
}
