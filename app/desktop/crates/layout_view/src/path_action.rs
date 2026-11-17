/// What a tab menu row does with the file a tab shows.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum PathAction {
    /// Put the file's full path on the clipboard.
    CopyPath,
    /// Put the file's path relative to its workspace on the clipboard.
    CopyRelativePath,
    /// Show the file in the system file manager.
    Reveal,
    /// Open a terminal in the file's folder.
    OpenInTerminal,
}

impl PathAction {
    pub(crate) const ALL: [Self; 4] = [
        Self::CopyPath,
        Self::CopyRelativePath,
        Self::Reveal,
        Self::OpenInTerminal,
    ];

    pub(crate) fn label(self) -> &'static str {
        match self {
            Self::CopyPath => "Copy Path",
            Self::CopyRelativePath => "Copy Relative Path",
            Self::Reveal if cfg!(target_os = "macos") => "Reveal in Finder",
            Self::Reveal => "Show in Files",
            Self::OpenInTerminal => "Open in Terminal",
        }
    }

    pub(crate) fn icon(self) -> ui::IconName {
        match self {
            Self::CopyPath | Self::CopyRelativePath => ui::IconName::Copy,
            Self::Reveal => ui::IconName::FolderOpen,
            Self::OpenInTerminal => ui::IconName::TerminalWindow,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn every_action_has_its_own_label() {
        let labels: Vec<&str> = PathAction::ALL
            .iter()
            .map(|action| action.label())
            .collect();
        let mut unique = labels.clone();
        unique.sort_unstable();
        unique.dedup();
        assert_eq!(unique.len(), labels.len(), "{labels:?}");
    }
}
