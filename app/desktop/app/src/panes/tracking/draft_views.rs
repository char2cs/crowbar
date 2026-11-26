use layout::{Command, Layout};

use crate::panes::tracking::open_chats::{open_chats, panes_showing};

// SPEC: docs/plans/2026-11-24-draft-thread.md#steps (step 5: a restored draft is dropped)
pub(crate) fn without_drafts(mut layout: Layout) -> Layout {
    let drafts: Vec<String> = open_chats(&layout)
        .into_iter()
        .filter(|chat| store::DraftThread::is_draft(chat))
        .collect();
    let panes = panes_showing(&layout, &drafts);
    if panes.is_empty() {
        return layout;
    }
    if let Err(error) = layout.apply(Command::ClosePanes { panes }) {
        tracing::warn!(%error, "could not drop the saved drafts");
    }
    layout
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::panes::kinds::core_panes::CHAT;
    use crate::test_support::{add_tab, chat_pane, one_tab, open_in};

    #[test]
    fn a_saved_draft_is_dropped_and_its_lone_tab_with_it() {
        let mut layout = one_tab([
            chat_pane(("a", CHAT), "c1"),
            chat_pane(("b", CHAT), "draft-u1"),
        ]);
        add_tab(&mut layout, 2);
        open_in(&mut layout, 2, chat_pane(("c", CHAT), "draft-u2"));
        let restored = without_drafts(layout);
        assert_eq!(open_chats(&restored), ["c1"], "only the real chat");
        assert_eq!(restored.tabs().len(), 1, "the draft's own tab closed");
    }
}
