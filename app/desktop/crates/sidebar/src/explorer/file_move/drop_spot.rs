use crate::shell::FileKind;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum DropSpot {
    Root,
    Row { path: String, kind: FileKind },
}

impl DropSpot {
    pub fn directory(&self) -> &str {
        match self {
            Self::Root => "",
            Self::Row {
                path,
                kind: FileKind::Directory(_),
            } => path,
            Self::Row { path, .. } => crate::shell::PathText::new(path).parent(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::shell::Expansion;

    fn row(
        path: &str,
        kind: FileKind,
    ) -> DropSpot {
        DropSpot::Row {
            path: path.into(),
            kind,
        }
    }

    #[test]
    fn a_folder_takes_the_drop_a_file_hands_it_to_its_folder_and_the_root_is_empty() {
        let cases = [
            (row("src", FileKind::Directory(Expansion::Collapsed)), "src"),
            (row("src/a.rs", FileKind::File), "src"),
            (row("README.md", FileKind::File), ""),
            (DropSpot::Root, ""),
        ];
        for (spot, want) in cases {
            assert_eq!(spot.directory(), want, "{spot:?}");
        }
    }
}
