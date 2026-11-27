//! Where a dragged row lands: a container row and an insert index among its other children.
//!
//! Spec: develop `web/src/components/sidebar/lib/drop-plan.ts`.

use sidebar::{DropMode, Row, RowTree};

use crate::plan::relocate::slot::Slot;

pub(crate) fn slot_for(
    rows: &[Row],
    (moved, target): (&str, &str),
    mode: DropMode,
) -> Option<Slot> {
    let at = |id: &str| rows.iter().position(|row| row.id.as_str() == id);
    let (subject, over) = (at(moved)?, at(target)?);
    let tree = RowTree::new(rows);
    let mut chain = std::iter::successors(Some(over), |index| tree.parent(*index));
    if chain.any(|index| index == subject) {
        return None;
    }
    let container = match mode {
        DropMode::Into => Some(over),
        DropMode::Before | DropMode::After => tree.parent(over),
    };
    let rest: Vec<usize> = tree.children(container).filter(|i| *i != subject).collect();
    let beside = rest.iter().position(|index| *index == over);
    let index = match mode {
        DropMode::Into => rest.len(),
        DropMode::Before => beside.unwrap_or(rest.len()),
        DropMode::After => beside.map_or(rest.len(), |at| at + 1),
    };
    let id = |index: Option<usize>| {
        let row = index.and_then(|index| rows.get(index));
        row.map_or_default(|row| row.id.as_str().to_owned())
    };
    Some(Slot {
        parent: id(container),
        from: id(tree.parent(subject)),
        index,
    })
}

#[cfg(test)]
mod tests {
    use sidebar::RowKind;

    use super::*;

    fn rows() -> Vec<Row> {
        let row = |id: &str, kind, depth| Row::new(id, kind, depth, id);
        vec![
            row("a", RowKind::Chat, 0),
            row("f", RowKind::Folder, 0),
            row("g", RowKind::Chat, 1),
            row("r", RowKind::RepoHeader, 0),
        ]
    }

    fn slot(
        rows_moved: (&str, &str),
        mode: DropMode,
    ) -> Option<(String, usize)> {
        slot_for(&rows(), rows_moved, mode).map(|slot| (slot.parent, slot.index))
    }

    #[test]
    fn before_and_after_count_among_the_siblings_with_the_moved_row_lifted_out() {
        let top = |index| Some((String::new(), index));
        assert_eq!(slot(("a", "r"), DropMode::Before), top(1));
        assert_eq!(slot(("a", "r"), DropMode::After), top(2), "a header counts");
        assert_eq!(slot(("r", "a"), DropMode::After), top(1));
        assert_eq!(slot(("a", "g"), DropMode::After), Some(("f".into(), 1)));
    }

    #[test]
    fn into_appends_to_the_container_and_a_row_cannot_go_into_itself() {
        assert_eq!(slot(("a", "f"), DropMode::Into), Some(("f".into(), 1)));
        assert_eq!(slot(("f", "g"), DropMode::Into), None);
        assert_eq!(slot(("a", "missing"), DropMode::Before), None);
    }
}
