//! What the Files panel's New File and New Folder entries create.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum NewEntry {
    File,
    Folder,
}

impl NewEntry {
    pub(crate) fn placeholder(self) -> &'static str {
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
