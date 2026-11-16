//! What happens to the branch once it is merged.

use ui::Pose;

/// Whether the merged branch and its chats are deleted.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum AfterMerge {
    /// The branch stays.
    #[default]
    Keep,
    /// The branch and its chats are deleted.
    DeleteBranch,
}

impl From<Pose> for AfterMerge {
    fn from(delete: Pose) -> Self {
        if delete == Pose::On {
            return Self::DeleteBranch;
        }
        Self::Keep
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_merge_keeps_the_branch_unless_told_otherwise() {
        assert_eq!(AfterMerge::default(), AfterMerge::Keep);
        assert_ne!(AfterMerge::DeleteBranch, AfterMerge::Keep);
    }

    #[test]
    fn the_delete_checkbox_maps_to_the_choice() {
        assert_eq!(AfterMerge::from(Pose::On), AfterMerge::DeleteBranch);
        assert_eq!(AfterMerge::from(Pose::Off), AfterMerge::Keep);
    }
}
