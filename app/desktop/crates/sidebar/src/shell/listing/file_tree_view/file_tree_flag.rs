//! The three switches of the Files panel's filter menu.

/// One switch of the filter menu; each is a stored setting.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord)]
pub enum FileTreeFlag {
    /// Dotfiles and dot-directories.
    Hidden,
    /// Files matched by `.gitignore`.
    Gitignored,
    /// The git status letters and colours.
    GitStatus,
}

impl FileTreeFlag {
    /// Every switch, in menu order.
    pub const ALL: [Self; 3] = [Self::Hidden, Self::Gitignored, Self::GitStatus];

    /// The setting key that stores the switch.
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
