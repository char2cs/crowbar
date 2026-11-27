#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum NewEntry {
    File,
    Folder,
}

impl NewEntry {
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
