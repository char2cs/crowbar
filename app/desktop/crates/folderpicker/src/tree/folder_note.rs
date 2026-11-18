//! What an opened folder shows in place of children it does not have yet.

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum FolderNote {
    Waiting,
    Loading,
    Empty,
    Failed(String),
    MoreFiles(usize),
}

impl FolderNote {
    pub(crate) fn text(
        &self,
        host: &str,
    ) -> String {
        match self {
            Self::Waiting => format!("Waiting for {host} to answer"),
            Self::Loading => "Loading…".to_owned(),
            Self::Empty => "Empty folder".to_owned(),
            Self::Failed(reason) => format!("Can't open: {reason}"),
            Self::MoreFiles(1) => "1 more file not shown".to_owned(),
            Self::MoreFiles(hidden) => format!("{hidden} more files not shown"),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: [(FolderNote, &str); 6] = [
        (FolderNote::Waiting, "Waiting for Box to answer"),
        (FolderNote::Loading, "Loading…"),
        (FolderNote::Empty, "Empty folder"),
        (FolderNote::MoreFiles(1), "1 more file not shown"),
        (FolderNote::MoreFiles(7), "7 more files not shown"),
        (FolderNote::Failed(String::new()), "Can't open: "),
    ];

    #[test]
    fn each_note_says_what_stands_in_for_the_children() {
        for (note, text) in CASES {
            assert_eq!(note.text("Box"), text);
        }
        assert_eq!(
            FolderNote::Failed("denied".into()).text("Box"),
            "Can't open: denied"
        );
    }
}
