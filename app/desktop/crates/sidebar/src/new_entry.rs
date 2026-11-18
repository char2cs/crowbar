//! What the Files panel's New File and New Folder entries create.

/// Whether the Files panel's inline input creates a file or a folder.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum NewEntry {
    /// A new file.
    File,
    /// A new folder.
    Folder,
}

impl NewEntry {
    /// The placeholder shown in the empty name input: `file-name` or `folder-name`.
    pub fn placeholder(self) -> &'static str {
        match self {
            Self::File => "file-name",
            Self::Folder => "folder-name",
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn each_kind_names_its_input() {
        assert_eq!(NewEntry::File.placeholder(), "file-name");
        assert_eq!(NewEntry::Folder.placeholder(), "folder-name");
    }
}
