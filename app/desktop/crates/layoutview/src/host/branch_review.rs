//! Whether a pane group's chat has a branch to review.

use layout::PaneInstance;

/// Whether a pane group's chat has a branch to review, which ends its tab row with develop's
/// "Review this branch" button; the shell answers it from the chat's worktree.
pub trait BranchReview {
    /// Whether the chat pane `chat` owns a branch to review. None does by default.
    fn reviews_branch(
        &self,
        _chat: &PaneInstance,
    ) -> bool {
        false
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::chat;

    struct Bare;

    impl BranchReview for Bare {}

    #[test]
    fn a_chat_has_no_branch_to_review_unless_the_shell_says_otherwise() {
        assert!(!Bare.reviews_branch(&chat("a")));
    }
}
