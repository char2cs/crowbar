//! The git state of a file as the trees decorate it.

/// A file's status letter and colour family.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum GitStatus {
    /// Unchanged.
    #[default]
    Clean,
    /// New and staged or tracked.
    Added,
    /// Changed.
    Modified,
    /// Removed.
    Deleted,
    /// Not tracked.
    Untracked,
    /// Matched by an ignore rule.
    Ignored,
}

impl GitStatus {
    /// The badge letter, none for clean files.
    pub fn letter(self) -> Option<char> {
        match self {
            Self::Clean | Self::Ignored => None,
            Self::Added => Some('A'),
            Self::Modified => Some('M'),
            Self::Deleted => Some('D'),
            Self::Untracked => Some('U'),
        }
    }
}

#[cfg(test)]
#[path = "git_status_tests.rs"]
mod tests;
