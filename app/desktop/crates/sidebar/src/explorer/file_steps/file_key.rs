#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FileKey {
    Up,
    Down,
    Home,
    End,
    Left,
    Right,
    Enter,
    Rename,
    Delete,
    Copy,
    Cut,
    Paste,
}

impl FileKey {
    pub fn with_modifier(key: &str) -> Option<Self> {
        match key {
            "c" => Some(Self::Copy),
            "x" => Some(Self::Cut),
            "v" => Some(Self::Paste),
            _ => None,
        }
    }

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
