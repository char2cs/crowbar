//! The files staged by Copy or Cut, waiting for a Paste.

use crate::clip_mode::ClipMode;
use crate::command::Command;
use crate::path_text::{base, join};

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct FileClipboard {
    pub(crate) root: Option<String>,
    pub(crate) paths: Vec<String>,
    pub(crate) mode: ClipMode,
}

impl FileClipboard {
    pub(crate) fn paste_into(
        &self,
        (root, dir): (&Option<String>, &str),
    ) -> Vec<Command> {
        if &self.root != root {
            return Vec::new();
        }
        self.paths
            .iter()
            .map(|path| self.command_for(path, join(dir, base(path))))
            .collect()
    }

    fn command_for(
        &self,
        path: &str,
        destination: String,
    ) -> Command {
        let path = path.to_owned();
        match self.mode {
            ClipMode::Copy => Command::CopyFile { path, destination },
            ClipMode::Cut => Command::RenameFile {
                path,
                new_path: destination,
            },
        }
    }

    pub(crate) fn survives_paste(&self) -> bool {
        self.mode == ClipMode::Copy
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn staged(
        mode: ClipMode,
        root: &str,
    ) -> FileClipboard {
        FileClipboard {
            root: Some(root.into()),
            paths: vec!["src/a.rs".into()],
            mode,
        }
    }

    #[test]
    fn pasting_copies_or_moves_into_the_directory() {
        let here = (&Some("/w".to_owned()), "docs");
        let copy = Command::CopyFile {
            path: "src/a.rs".into(),
            destination: "docs/a.rs".into(),
        };
        let moved = Command::RenameFile {
            path: "src/a.rs".into(),
            new_path: "docs/a.rs".into(),
        };
        assert_eq!(staged(ClipMode::Copy, "/w").paste_into(here), vec![copy]);
        assert_eq!(staged(ClipMode::Cut, "/w").paste_into(here), vec![moved]);
    }

    #[test]
    fn another_workspace_pastes_nothing_and_only_a_copy_stays_staged() {
        let elsewhere = (&Some("/other".to_owned()), "docs");
        let nothing = staged(ClipMode::Copy, "/w").paste_into(elsewhere);
        assert_eq!(nothing, Vec::<Command>::new());
        assert!(staged(ClipMode::Copy, "/w").survives_paste());
        assert!(!staged(ClipMode::Cut, "/w").survives_paste());
    }
}
