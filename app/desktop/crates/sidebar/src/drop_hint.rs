//! The placement the pointer currently selects.

use crate::drag_point::DragPoint;
use crate::drop_mode::DropMode;
use crate::drop_policy::allowed_modes;
use crate::folder_target::FolderTarget;
use crate::row::Row;
use crate::row_id::RowId;
use crate::row_kind::RowKind;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct DropHint {
    pub(crate) row: RowId,
    pub(crate) mode: DropMode,
}

impl DropHint {
    pub(crate) fn resolve(
        rows: &[Row],
        point: &DragPoint,
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
        let mode = allowed_modes(rows, subject, target).at(ratio, folder)?;
        Some(Self {
            row: point.target.clone(),
            mode,
        })
    }
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
            let got = DropHint::resolve(&rows(), &over(subject, target, y)).map(|hint| hint.mode);
            assert_eq!(got, want, "{subject} over {target} at {y}");
        }
    }
}
