//! The placement the pointer currently selects.

use crate::drag_point::DragPoint;
use crate::drop_mode::DropMode;
use crate::drop_modes::DropModes;
use crate::folder_target::FolderTarget;
use crate::nesting::Nesting;
use crate::row::Row;
use crate::row_id::RowId;
use crate::row_kind::RowKind;

#[derive(Debug, Clone, PartialEq)]
pub(crate) struct DropHint {
    pub(crate) row: RowId,
    pub(crate) mode: DropMode,
}

impl DropHint {
    pub(crate) fn resolve(
        rows: &[Row],
        point: &DragPoint,
        folded: impl Fn(&RowId) -> bool,
    ) -> Option<Self> {
        let index = |id: &RowId| rows.iter().position(|row| &row.id == id);
        let (subject, target) = (index(&point.subject)?, index(&point.target)?);
        let height = f32::from(point.bounds.size.height);
        if height <= 0.0 {
            return None;
        }
        let ratio = f32::from(point.at.y - point.bounds.origin.y) / height;
        let folder = match rows.get(target)?.kind {
            RowKind::Folder => FolderTarget::Yes,
            _ => FolderTarget::No,
        };
        let mode = DropModes::allowed(rows, subject, target).at(ratio, folder)?;
        let hint = Self {
            row: point.target.clone(),
            mode,
        };
        let reparents = mode == DropMode::After && opens_children(rows, target, &folded);
        if !reparents {
            return Some(hint);
        }
        Self::first_child(rows, (subject, target))
    }

    fn first_child(
        rows: &[Row],
        (subject, parent): (usize, usize),
    ) -> Option<Self> {
        let child = rows.get(parent + 1)?;
        let leaves_alone = rows.get(subject).is_some_and(Row::is_ownerless);
        let allowed = DropModes::allowed(rows, subject, parent + 1).contains(DropMode::Before);
        if leaves_alone || !allowed {
            return None;
        }
        Some(Self {
            row: child.id.clone(),
            mode: DropMode::Before,
        })
    }
}

fn opens_children(
    rows: &[Row],
    index: usize,
    folded: &impl Fn(&RowId) -> bool,
) -> bool {
    let Some(row) = rows.get(index) else {
        return false;
    };
    let below = rows
        .get(index + 1)
        .is_some_and(|next| next.depth > row.depth);
    row.nesting == Nesting::Parent && below && !folded(&row.id)
}

#[cfg(test)]
mod tests {
    use gpui::{Bounds, point, px, size};

    use super::*;

    type Case = (&'static str, &'static str, f32, Option<DropMode>);

    fn rows() -> Vec<Row> {
        vec![
            Row::new("a", RowKind::Chat, 1, "a"),
            Row::new("b", RowKind::Chat, 1, "b"),
            Row::new("f", RowKind::Folder, 1, "f"),
        ]
    }

    fn over(
        subject: &str,
        target: &str,
        y: f32,
    ) -> DragPoint {
        DragPoint {
            subject: RowId::from(subject),
            target: RowId::from(target),
            bounds: Bounds::new(point(px(0.0), px(100.0)), size(px(200.0), px(36.0))),
            at: point(px(10.0), px(100.0 + y)),
        }
    }

    #[test]
    fn the_pointer_height_picks_the_placement() {
        const CASES: [Case; 7] = [
            ("a", "b", 2.0, Some(DropMode::Before)),
            ("a", "b", 18.0, Some(DropMode::After)),
            ("a", "b", 34.0, Some(DropMode::After)),
            ("a", "f", 5.0, Some(DropMode::Before)),
            ("a", "f", 18.0, Some(DropMode::Into)),
            ("a", "a", 18.0, None),
            ("a", "missing", 18.0, None),
        ];
        for (subject, target, y, want) in CASES {
            let got = DropHint::resolve(&rows(), &over(subject, target, y), |_| false)
                .map(|hint| hint.mode);
            assert_eq!(got, want, "{subject} over {target} at {y}");
        }
    }

    fn family() -> Vec<Row> {
        let mut parent = Row::new("p", RowKind::Folder, 1, "p");
        parent.nesting = Nesting::Parent;
        vec![
            Row::new("a", RowKind::Chat, 1, "a"),
            parent,
            Row::new("c", RowKind::Chat, 2, "c"),
        ]
    }

    #[test]
    fn after_an_expanded_parent_is_its_first_child_slot() {
        let point = over("a", "p", 35.0);
        let open = DropHint::resolve(&family(), &point, |_| false).expect("a slot");
        assert_eq!(
            (open.row, open.mode),
            (RowId::from("c"), DropMode::Before),
            "open"
        );
        let shut = DropHint::resolve(&family(), &point, |_| true).expect("a slot");
        assert_eq!(
            (shut.row, shut.mode),
            (RowId::from("p"), DropMode::After),
            "folded"
        );
    }
}
