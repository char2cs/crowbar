//! Which of develop's two add dialogs a view is.

/// Which of develop's two add dialogs a view is.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Kind {
    /// "Add a space" (develop's `ImportProjectModal`): the folder becomes a new space.
    Space,
    /// "Add repository" (develop's `AddRepositoryModal`): the folder joins the open space.
    Repo,
}

impl Kind {
    pub(crate) fn title(self) -> &'static str {
        match self {
            Self::Space => "Add a space",
            Self::Repo => "Add repository",
        }
    }

    pub(crate) fn folder_label(self) -> &'static str {
        match self {
            Self::Space => "Space folder",
            Self::Repo => "Repository folder",
        }
    }

    pub(crate) fn folder_placeholder(self) -> &'static str {
        match self {
            Self::Space => "/absolute/path/to/folder",
            Self::Repo => "/absolute/path/to/repo",
        }
    }

    pub(crate) fn name_label(self) -> &'static str {
        match self {
            Self::Space => "Space name",
            Self::Repo => "Repository name",
        }
    }

    pub(crate) fn name_note(self) -> Option<&'static str> {
        match self {
            Self::Space => None,
            Self::Repo => Some("(optional)"),
        }
    }

    pub(crate) fn name_placeholder(self) -> &'static str {
        match self {
            Self::Space => "My space",
            Self::Repo => "Derived from folder name",
        }
    }

    pub(crate) fn submit(self) -> &'static str {
        match self {
            Self::Space => "Add space",
            Self::Repo => "Add repository",
        }
    }

    pub(crate) fn names_from_browse(self) -> bool {
        self == Self::Repo
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn each_dialog_carries_develop_s_words() {
        let words = |kind: Kind| (kind.title(), kind.folder_label(), kind.submit());
        assert_eq!(
            words(Kind::Space),
            ("Add a space", "Space folder", "Add space")
        );
        assert_eq!(
            words(Kind::Repo),
            ("Add repository", "Repository folder", "Add repository")
        );
        assert_eq!(Kind::Repo.name_note(), Some("(optional)"));
        assert_eq!(Kind::Space.name_note(), None);
    }
}
