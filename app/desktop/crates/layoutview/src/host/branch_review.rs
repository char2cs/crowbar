use layout::PaneInstance;

pub trait BranchReview {
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
