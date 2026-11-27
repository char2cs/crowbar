#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum GitStatus {
    #[default]
    Clean,
    Added,
    Modified,
    StagedModified,
    Renamed,
    Deleted,
    Untracked,
    Ignored,
}

impl GitStatus {
    pub fn letter(self) -> Option<char> {
        match self {
            Self::Clean | Self::Ignored => None,
            Self::Added => Some('A'),
            Self::Modified | Self::StagedModified => Some('M'),
            Self::Renamed => Some('R'),
            Self::Deleted => Some('D'),
            Self::Untracked => Some('U'),
        }
    }

    #[must_use]
    pub fn priority(self) -> usize {
        ROLL_UP
            .iter()
            .position(|status| *status == self)
            .map_or(0, |rank| rank.saturating_add(1))
    }
}

const ROLL_UP: [GitStatus; 6] = [
    GitStatus::Untracked,
    GitStatus::Added,
    GitStatus::Renamed,
    GitStatus::Modified,
    GitStatus::StagedModified,
    GitStatus::Deleted,
];

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn letters_follow_the_status() {
        let cases = [
            (GitStatus::Clean, None),
            (GitStatus::Ignored, None),
            (GitStatus::Added, Some('A')),
            (GitStatus::Modified, Some('M')),
            (GitStatus::Deleted, Some('D')),
            (GitStatus::Untracked, Some('U')),
            (GitStatus::Renamed, Some('R')),
            (GitStatus::StagedModified, Some('M')),
        ];
        for (status, letter) in cases {
            assert_eq!(status.letter(), letter, "{status:?}");
        }
    }

    #[test]
    fn a_deletion_outranks_every_other_change_and_untracked_ranks_lowest() {
        let ranked = [
            GitStatus::Untracked,
            GitStatus::Added,
            GitStatus::Renamed,
            GitStatus::Modified,
            GitStatus::StagedModified,
            GitStatus::Deleted,
        ];
        let priorities: Vec<usize> = ranked.iter().map(|status| status.priority()).collect();
        assert!(
            priorities.windows(2).all(|pair| pair[0] < pair[1]),
            "{priorities:?}"
        );
        assert_eq!(GitStatus::Clean.priority(), 0);
    }
}
