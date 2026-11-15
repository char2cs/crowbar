//! How a branch is merged into its parent.

/// The three ways the daemon merges a workspace into its parent.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum MergeStrategy {
    /// A merge commit.
    #[default]
    Merge,
    /// All commits folded into one.
    Squash,
    /// The commits replayed onto the parent.
    Rebase,
}

impl MergeStrategy {
    /// Every strategy, in the order the select lists them.
    pub const ALL: [Self; 3] = [Self::Merge, Self::Squash, Self::Rebase];

    /// The select's option text.
    #[must_use]
    pub const fn label(self) -> &'static str {
        match self {
            Self::Merge => "Create a merge commit",
            Self::Squash => "Squash and merge",
            Self::Rebase => "Rebase and merge",
        }
    }

    /// The confirm button's text.
    #[must_use]
    pub const fn confirm(self) -> &'static str {
        match self {
            Self::Merge => "Create merge commit",
            Self::Squash => "Squash & merge",
            Self::Rebase => "Rebase & merge",
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn each_strategy_has_its_own_label_and_confirm_text() {
        let labels: Vec<_> = MergeStrategy::ALL
            .iter()
            .map(|s| (s.label(), s.confirm()))
            .collect();
        assert_eq!(labels[0], ("Create a merge commit", "Create merge commit"));
        assert_eq!(labels[1], ("Squash and merge", "Squash & merge"));
        assert_eq!(labels[2], ("Rebase and merge", "Rebase & merge"));
        assert_eq!(MergeStrategy::default(), MergeStrategy::Merge);
    }
}
