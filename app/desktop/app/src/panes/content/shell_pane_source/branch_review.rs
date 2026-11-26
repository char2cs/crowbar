use layout::PaneInstance;

use super::{ChatReviews, ShellPaneSource};
use crate::panes::ide::tab_looks;

impl ShellPaneSource {
    pub(crate) fn reviewing(
        mut self,
        reviews: ChatReviews,
    ) -> Self {
        self.reviews = reviews;
        self
    }
}

impl layoutview::BranchReview for ShellPaneSource {
    fn reviews_branch(
        &self,
        chat: &PaneInstance,
    ) -> bool {
        tab_looks::chat_of(chat).is_some_and(|chat| (self.reviews)(chat))
    }
}

#[cfg(test)]
mod tests {
    use layoutview::BranchReview as _;

    use crate::panes::content::shell_pane_source::tests::idle_source;
    use crate::panes::tracking::titles::Titles;
    use crate::test_support::chat_pane;

    #[test]
    fn only_a_chat_pane_whose_chat_owns_a_branch_offers_a_review() {
        let source =
            idle_source(Titles::default()).reviewing(std::rc::Rc::new(|chat| chat == "owner"));
        let cases = [
            ("core.chat", "owner", true),
            ("core.chat", "thread", false),
            ("core.terminal", "owner", false),
        ];
        for (kind, thread, expected) in cases {
            assert_eq!(
                source.reviews_branch(&chat_pane(("p1", kind), thread)),
                expected,
                "{kind} {thread}"
            );
        }
    }
}
