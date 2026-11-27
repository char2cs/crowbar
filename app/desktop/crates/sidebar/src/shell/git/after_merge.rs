#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum AfterMerge {
    #[default]
    Keep,
    DeleteBranch,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_merge_keeps_the_branch_unless_told_otherwise() {
        assert_eq!(AfterMerge::default(), AfterMerge::Keep);
        assert_ne!(AfterMerge::DeleteBranch, AfterMerge::Keep);
    }
}
