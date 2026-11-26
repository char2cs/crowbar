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
    use protocol::PaneId;

    use super::*;
    use crate::panes::browser::browser_bars::BrowserBars;
    use crate::panes::content::native_panes::NativePanes;
    use crate::panes::content::new_tab::new_tab_panes::NewTabPanes;
    use crate::panes::kinds::serials::Serials;
    use crate::panes::terminals::terminal_ids::TerminalIds;
    use crate::panes::tracking::titles::Titles;
    use crate::test_support::idle_terminals;

    fn pane(
        kind: &str,
        thread: &str,
    ) -> PaneInstance {
        PaneInstance {
            pane_id: PaneId::new("p1").unwrap(),
            type_id: protocol::PaneTypeId::new(kind).unwrap(),
            params: protocol::Params::default()
                .with("threadId", thread)
                .unwrap(),
            state: protocol::PaneState::default(),
        }
    }

    #[test]
    fn only_a_chat_pane_whose_chat_owns_a_branch_offers_a_review() {
        let source = ShellPaneSource::new(
            Titles::default(),
            Serials::above(&layout::Layout::default()),
            BrowserBars::new(
                crate::panes::browser::browser_ids::BrowserIds::default(),
                async_channel::unbounded().0,
            ),
            NativePanes::new(
                idle_terminals(TerminalIds::default()),
                NewTabPanes::new(async_channel::unbounded().0),
                crate::test_support::idle_editors(),
            ),
        )
        .reviewing(std::rc::Rc::new(|chat| chat == "owner"));
        let cases = [
            ("core.chat", "owner", true),
            ("core.chat", "thread", false),
            ("core.terminal", "owner", false),
        ];
        for (kind, thread, expected) in cases {
            assert_eq!(
                source.reviews_branch(&pane(kind, thread)),
                expected,
                "{kind} {thread}"
            );
        }
    }
}
