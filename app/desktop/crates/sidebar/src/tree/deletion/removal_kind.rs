#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RemovalKind {
    Project,
    Repo,
    Workspace,
    Chat,
    Folder,
}

impl RemovalKind {
    pub(crate) fn noun(self) -> &'static str {
        match self {
            Self::Project => "space",
            Self::Repo => "repository",
            Self::Workspace => "workspace",
            Self::Chat => "chat",
            Self::Folder => "folder",
        }
    }

    pub(crate) fn needs_confirmation(self) -> bool {
        matches!(self, Self::Project | Self::Repo)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_repositories_and_spaces_ask_first() {
        let cases = [
            (RemovalKind::Project, "space", true),
            (RemovalKind::Repo, "repository", true),
            (RemovalKind::Workspace, "workspace", false),
            (RemovalKind::Chat, "chat", false),
            (RemovalKind::Folder, "folder", false),
        ];
        for (kind, noun, confirms) in cases {
            assert_eq!(kind.noun(), noun, "{kind:?} noun");
            assert_eq!(kind.needs_confirmation(), confirms, "{kind:?} confirmation");
        }
    }
}
