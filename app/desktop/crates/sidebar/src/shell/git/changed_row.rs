use crate::shell::Expansion;
use crate::shell::git::changed_file::ChangedFile;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ChangedRow {
    Folder {
        path: String,
        label: String,
        depth: u8,
        expansion: Expansion,
        files: Vec<(String, String)>,
        viewed: usize,
    },
    File {
        file: ChangedFile,
        name: String,
        depth: u8,
        threads: u32,
        viewed: bool,
    },
}

impl ChangedRow {
    #[must_use]
    pub fn depth(&self) -> u8 {
        match self {
            Self::Folder { depth, .. } | Self::File { depth, .. } => *depth,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_folder_and_a_file_report_their_level() {
        let folder = ChangedRow::Folder {
            path: "src".into(),
            label: "src".into(),
            depth: 1,
            expansion: Expansion::Expanded,
            files: Vec::new(),
            viewed: 0,
        };
        let file = ChangedRow::File {
            file: ChangedFile::default(),
            name: "a.ts".into(),
            depth: 2,
            threads: 0,
            viewed: false,
        };
        assert_eq!((folder.depth(), file.depth()), (1, 2));
    }
}
