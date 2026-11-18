//! The files staged by Copy or Cut, waiting for a Paste.

use crate::shell::ClipMode;
use crate::shell::Command;
use crate::sidebar::{base, join};

/// Files staged in one workspace, to be copied or moved on Paste.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct FileClipboard {
    /// The root of the workspace the files were staged in.
    pub root: Option<String>,
    /// The staged files, relative to that root.
    pub paths: Vec<String>,
    /// Whether Paste copies or moves them.
    pub mode: ClipMode,
}

impl FileClipboard {
    /// The commands that copy or move every staged file into `dir` of the workspace at `root`.
    /// A workspace other than the one the files were staged in gets none.
    pub fn paste_into(
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

    /// Whether the files stay staged after a Paste: a copy does, a cut does not.
    pub fn survives_paste(&self) -> bool {
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
