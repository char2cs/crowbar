use gpui::Context;
use layout::PaneGroup;
use protocol::PaneId;

use crate::layout_event::LayoutEvent;
use crate::layout_view::LayoutView;
use ideshell::TabAction;

impl LayoutView {
    pub(crate) fn review_action(
        &self,
        (group, anchor): (&PaneGroup, &PaneId),
        cx: &mut Context<'_, Self>,
    ) -> Option<TabAction> {
        let chat = group.chat()?;
        let instance = self.tab.as_ref()?.pane(chat)?;
        if !self.source.reviews_branch(instance) {
            return None;
        }
        let review = LayoutEvent::ReviewBranch {
            onto: anchor.clone(),
        };
        Some(Self::emitting(review, cx))
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use layout::Command;

    use super::*;
    use crate::test_support::{FakeSource, editor, layout_of, open_with, pane_id, strip_of};

    fn chat_with_a_file() -> layout::Layout {
        let mut layout = layout_of("a", &[]);
        let open = Command::OpenPaneInGroup {
            instance: editor("b"),
            onto: pane_id("a"),
            index: None,
        };
        layout.apply(open).expect("b beside chat a");
        layout
    }

    #[gpui::test]
    fn a_chat_with_a_branch_ends_its_row_in_a_review_shortcut_for_its_group(
        cx: &mut TestAppContext
    ) {
        let source = FakeSource::starting_at_first_tab().reviewing(pane_id("a"));
        let harness = open_with(cx, &chat_with_a_file(), source);
        let (view, cx, opened) = (harness.view, harness.cx, harness.opened);
        let strip = view
            .update(cx, |view, cx| strip_of(view, &pane_id("a"), cx))
            .expect("strip");
        let review = strip.review.expect("a review shortcut");
        cx.update(|window, cx| review(window, cx));
        assert_eq!(
            opened.borrow().as_slice(),
            &[LayoutEvent::ReviewBranch { onto: pane_id("b") }],
            "the shown tab is b"
        );
    }

    #[gpui::test]
    fn a_chat_with_no_branch_has_no_review_shortcut(cx: &mut TestAppContext) {
        let source = FakeSource::starting_at_first_tab();
        let harness = open_with(cx, &chat_with_a_file(), source);
        let (view, cx) = (harness.view, harness.cx);
        let strip = view
            .update(cx, |view, cx| strip_of(view, &pane_id("a"), cx))
            .expect("strip");
        assert!(strip.review.is_none(), "a thread or home chat");
    }
}
