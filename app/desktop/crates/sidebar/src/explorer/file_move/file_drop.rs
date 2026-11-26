//! Where a dragged Files row would land if it were released now.

use crate::explorer::file_move::drop_spot::DropSpot;
use crate::shell::{Expansion, FileKind};

/// A spot that accepts the dragged row: the highlighted row, or the root, and the folder the row
/// lands in.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct FileDrop {
    spot: DropSpot,
}

impl FileDrop {
    pub(crate) fn new(spot: DropSpot) -> Self {
        Self { spot }
    }

    /// The spot under the pointer.
    pub fn spot(&self) -> &DropSpot {
        &self.spot
    }

    /// Whether the row at `path` is drawn as the target (a file row is, though its folder
    /// receives the drop, as develop draws it).
    pub fn marks(
        &self,
        path: &str,
    ) -> bool {
        matches!(&self.spot, DropSpot::Row { path: over, .. } if over == path)
    }

    /// Whether the panel itself is the target, drawn with develop's dashed outline.
    pub fn is_root(&self) -> bool {
        self.spot == DropSpot::Root
    }

    /// The folder the row lands in.
    pub fn directory(&self) -> &str {
        self.spot.directory()
    }

    /// The folded folder that unfolds when the pointer rests on it.
    pub fn unfolds(&self) -> Option<&str> {
        match &self.spot {
            DropSpot::Row {
                path,
                kind: FileKind::Directory(Expansion::Collapsed),
            } => Some(path),
            _ => None,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn over(
        path: &str,
        kind: FileKind,
    ) -> FileDrop {
        FileDrop::new(DropSpot::Row {
            path: path.into(),
            kind,
        })
    }

    #[test]
    fn the_hovered_row_is_marked_and_only_a_folded_folder_unfolds() {
        let folded = FileKind::Directory(Expansion::Collapsed);
        let open = FileKind::Directory(Expansion::Expanded);
        let cases = [
            (over("src", folded), Some("src"), Some("src")),
            (over("src", open), Some("src"), None),
            (over("src/a.rs", FileKind::File), Some("src/a.rs"), None),
            (FileDrop::new(DropSpot::Root), None, None),
        ];
        for (target, marked, unfolds) in cases {
            let marks = |path: &str| target.marks(path);
            assert_eq!(marked.is_some_and(marks), marked.is_some(), "{target:?}");
            assert!(!target.marks("other"), "{target:?}");
            assert_eq!(target.unfolds(), unfolds, "{target:?}");
            assert_eq!(target.is_root(), marked.is_none(), "{target:?}");
        }
    }
}
