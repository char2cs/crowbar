//! What a "new" file operation makes.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum NewEntry {
    /// An empty file.
    File,
    /// An empty directory.
    Directory,
}

impl NewEntry {
    pub(crate) fn kind(self) -> &'static str {
        match self {
            Self::File => "file",
            Self::Directory => "dir",
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_daemon_names_a_new_file_and_a_new_directory() {
        assert_eq!(NewEntry::File.kind(), "file");
        assert_eq!(NewEntry::Directory.kind(), "dir");
    }
}
