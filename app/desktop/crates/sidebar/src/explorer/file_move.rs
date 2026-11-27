pub(crate) mod drop_spot;
pub(crate) mod file_drop;

use crate::explorer::file_move::drop_spot::DropSpot;
use crate::explorer::file_move::file_drop::FileDrop;
use crate::shell::{Command, FileKind, PathText};

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct FileMove {
    path: String,
    kind: FileKind,
}

impl FileMove {
    pub fn new(
        path: &str,
        kind: FileKind,
    ) -> Self {
        Self {
            path: path.to_owned(),
            kind,
        }
    }

    pub fn path(&self) -> &str {
        &self.path
    }

    pub(crate) fn is_folder(&self) -> bool {
        matches!(self.kind, FileKind::Directory(_))
    }

    pub fn onto(
        &self,
        spot: DropSpot,
    ) -> Option<FileDrop> {
        let refused = match &spot {
            DropSpot::Root => false,
            DropSpot::Row { path, .. } => path == &self.path || self.holds(path),
        };
        (!refused).then(|| FileDrop::new(spot))
    }

    pub fn command(
        &self,
        target: &FileDrop,
    ) -> Option<Command> {
        let name = PathText::new(&self.path).base();
        let new_path = PathText::new(target.directory()).join(name);
        (new_path != self.path).then(|| Command::MoveFile {
            path: self.path.clone(),
            new_path,
        })
    }

    fn holds(
        &self,
        path: &str,
    ) -> bool {
        self.is_folder() && path.starts_with(&format!("{}/", self.path))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::shell::Expansion;

    const FOLDER: FileKind = FileKind::Directory(Expansion::Collapsed);

    fn row(
        path: &str,
        kind: FileKind,
    ) -> DropSpot {
        DropSpot::Row {
            path: path.into(),
            kind,
        }
    }

    #[derive(Debug, PartialEq)]
    enum Landing {
        Refused,
        InPlace,
        To(&'static str),
    }

    fn landing(
        dragged: &FileMove,
        spot: DropSpot,
    ) -> Option<String> {
        let target = dragged.onto(spot)?;
        let Some(command) = dragged.command(&target) else {
            return Some(String::new());
        };
        match command {
            Command::MoveFile { new_path, .. } => Some(new_path),
            other => panic!("a move, not {other:?}"),
        }
    }

    fn expected(want: &Landing) -> Option<String> {
        match want {
            Landing::Refused => None,
            Landing::InPlace => Some(String::new()),
            Landing::To(path) => Some((*path).to_owned()),
        }
    }

    #[test]
    fn a_drop_moves_into_the_folder_beside_the_file_or_to_the_root() {
        let file = FileMove::new("src/a.rs", FileKind::File);
        let folder = FileMove::new("src", FOLDER);
        let cases = [
            (&file, row("lib", FOLDER), Landing::To("lib/a.rs")),
            (
                &file,
                row("lib/b.rs", FileKind::File),
                Landing::To("lib/a.rs"),
            ),
            (&file, DropSpot::Root, Landing::To("a.rs")),
            (&file, row("src", FOLDER), Landing::InPlace),
            (&file, row("src/b.rs", FileKind::File), Landing::InPlace),
            (&file, row("src/a.rs", FileKind::File), Landing::Refused),
            (&folder, row("lib", FOLDER), Landing::To("lib/src")),
            (&folder, row("src", FOLDER), Landing::Refused),
            (&folder, row("src/inner", FOLDER), Landing::Refused),
            (&folder, row("src/a.rs", FileKind::File), Landing::Refused),
            (&folder, row("srcs", FOLDER), Landing::To("srcs/src")),
            (&folder, DropSpot::Root, Landing::InPlace),
        ];
        for (dragged, spot, want) in cases {
            let got = landing(dragged, spot.clone());
            assert_eq!(
                got,
                expected(&want),
                "{} onto {spot:?}: {want:?}",
                dragged.path()
            );
        }
    }
}
