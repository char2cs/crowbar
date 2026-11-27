use crate::explorer::file_move::drop_spot::DropSpot;
use crate::shell::{Expansion, FileKind};

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct FileDrop {
    spot: DropSpot,
}

impl FileDrop {
    pub(crate) fn new(spot: DropSpot) -> Self {
        Self { spot }
    }

    pub fn spot(&self) -> &DropSpot {
        &self.spot
    }

    pub fn marks(
        &self,
        path: &str,
    ) -> bool {
        matches!(&self.spot, DropSpot::Row { path: over, .. } if over == path)
    }

    pub fn is_root(&self) -> bool {
        self.spot == DropSpot::Root
    }

    pub fn directory(&self) -> &str {
        self.spot.directory()
    }

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
