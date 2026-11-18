//! The keys the Files panel answers when it has focus.

/// A key the focused Files panel acts on.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FileKey {
    /// Selects the row above.
    Up,
    /// Selects the row below.
    Down,
    /// Selects the first row.
    Home,
    /// Selects the last row.
    End,
    /// Folds an open directory, or selects the parent directory.
    Left,
    /// Unfolds a folded directory, or enters an open one.
    Right,
    /// Opens a file or toggles a directory.
    Enter,
    /// F2: starts renaming the selected row.
    Rename,
    /// Asks to delete the selected row.
    Delete,
    /// Stages the selected row to be copied.
    Copy,
    /// Stages the selected row to be moved.
    Cut,
    /// Pastes the staged files into the selected directory.
    Paste,
}

impl FileKey {
    /// The key meant by `key` pressed with the secondary modifier: `c`, `x` or `v`.
    pub fn with_modifier(key: &str) -> Option<Self> {
        match key {
            "c" => Some(Self::Copy),
            "x" => Some(Self::Cut),
            "v" => Some(Self::Paste),
            _ => None,
        }
    }

    /// The key meant by `key` pressed without a modifier.
    /// Arrows, Home, End, Enter, F2 and Delete answer; any other key is `None`.
    pub fn plain(key: &str) -> Option<Self> {
        match key {
            "up" => Some(Self::Up),
            "down" => Some(Self::Down),
            "home" => Some(Self::Home),
            "end" => Some(Self::End),
            "left" => Some(Self::Left),
            "right" => Some(Self::Right),
            "enter" => Some(Self::Enter),
            "f2" => Some(Self::Rename),
            "delete" => Some(Self::Delete),
            _ => None,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn plain_keys_navigate_and_the_secondary_modifier_stages_files() {
        type Case = (&'static str, Option<FileKey>, Option<FileKey>);
        const CASES: [Case; 5] = [
            ("down", Some(FileKey::Down), None),
            ("f2", Some(FileKey::Rename), None),
            ("delete", Some(FileKey::Delete), None),
            ("c", None, Some(FileKey::Copy)),
            ("v", None, Some(FileKey::Paste)),
        ];
        for (key, plain, modified) in CASES {
            assert_eq!(FileKey::plain(key), plain, "plain {key}");
            assert_eq!(FileKey::with_modifier(key), modified, "modified {key}");
        }
    }
}
