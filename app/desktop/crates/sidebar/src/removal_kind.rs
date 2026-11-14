//! What a removal deletes.

/// The noun of a removal; repositories and spaces ask before deleting, the rest count down.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RemovalKind {
    /// A whole space.
    Project,
    /// A repository.
    Repo,
    /// A branch workspace.
    Workspace,
    /// A chat.
    Chat,
    /// A folder.
    Folder,
}

impl RemovalKind {
    /// The word used in titles and buttons.
    pub fn noun(self) -> &'static str {
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
#[path = "removal_kind_tests.rs"]
mod tests;
