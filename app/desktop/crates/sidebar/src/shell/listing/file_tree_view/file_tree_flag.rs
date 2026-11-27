#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord)]
pub enum FileTreeFlag {
    Hidden,
    Gitignored,
    GitStatus,
}

impl FileTreeFlag {
    pub const ALL: [Self; 3] = [Self::Hidden, Self::Gitignored, Self::GitStatus];

    pub fn setting_key(self) -> &'static str {
        match self {
            Self::Hidden => "showHiddenFilesInFileTree",
            Self::Gitignored => "showGitignoredFilesInFileTree",
            Self::GitStatus => "showGitStatusInFileTree",
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn each_switch_has_its_own_setting_key() {
        let keys = [
            FileTreeFlag::Hidden,
            FileTreeFlag::Gitignored,
            FileTreeFlag::GitStatus,
        ]
        .map(FileTreeFlag::setting_key);
        assert_eq!(
            keys,
            [
                "showHiddenFilesInFileTree",
                "showGitignoredFilesInFileTree",
                "showGitStatusInFileTree"
            ]
        );
    }
}
