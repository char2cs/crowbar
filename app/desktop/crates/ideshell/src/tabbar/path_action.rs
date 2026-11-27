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

const SCHEME_SEPARATOR: &str = "://";
const VIRTUAL_ACTIONS: usize = 3;

impl PathAction {
    /// Every action, in menu order.
    pub const ALL: [Self; 4] = [
        Self::CopyPath,
        Self::CopyRelativePath,
        Self::Reveal,
        Self::OpenInTerminal,
    ];

    /// The actions a tab menu offers for `path`: a virtual path has no folder to open a terminal
    /// in (develop's `tab-context-menu.tsx`).
    #[must_use]
    pub fn offered_for(path: &str) -> &'static [Self] {
        if Self::is_virtual(path) {
            return &Self::ALL[..VIRTUAL_ACTIONS];
        }
        &Self::ALL
    }

    /// Whether `path` names no file on disk, as a terminal tab's `terminal://<pane>` does.
    #[must_use]
    pub fn is_virtual(path: &str) -> bool {
        path.contains(SCHEME_SEPARATOR)
    }

    /// The menu row's text.
    #[must_use]
    pub fn label(self) -> &'static str {
        match self {
            Self::CopyPath => "Copy Path",
            Self::CopyRelativePath => "Copy Relative Path",
            Self::Reveal => "Reveal in Finder",
            Self::OpenInTerminal => "Open in Terminal",
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_virtual_path_offers_no_terminal_in_its_folder() {
        let cases = [
            ("src/main.rs", PathAction::ALL.as_slice()),
            (
                "terminal://t1",
                [
                    PathAction::CopyPath,
                    PathAction::CopyRelativePath,
                    PathAction::Reveal,
                ]
                .as_slice(),
            ),
        ];
        for (path, offered) in cases {
            assert_eq!(PathAction::offered_for(path), offered, "{path}");
        }
    }

    #[test]
    fn reveal_keeps_develops_label_on_every_desktop() {
        assert_eq!(PathAction::Reveal.label(), "Reveal in Finder");
    }

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
