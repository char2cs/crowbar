#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum PathAction {
    CopyPath,
    CopyRelativePath,
    Reveal,
    OpenInTerminal,
}

const SCHEME_SEPARATOR: &str = "://";
const VIRTUAL_ACTIONS: usize = 3;

impl PathAction {
    pub const ALL: [Self; 4] = [
        Self::CopyPath,
        Self::CopyRelativePath,
        Self::Reveal,
        Self::OpenInTerminal,
    ];

    #[must_use]
    pub fn offered_for(path: &str) -> &'static [Self] {
        if Self::is_virtual(path) {
            return &Self::ALL[..VIRTUAL_ACTIONS];
        }
        &Self::ALL
    }

    #[must_use]
    pub fn is_virtual(path: &str) -> bool {
        path.contains(SCHEME_SEPARATOR)
    }

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
