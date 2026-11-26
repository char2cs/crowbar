use std::collections::BTreeSet;

use layout::{Layout, PaneInstance};
use protocol::PaneId;

const CHAT_ID: &str = "threadId";

pub(crate) fn open_chats(layout: &Layout) -> Vec<String> {
    chat_panes(layout)
        .into_iter()
        .map(|(_, chat)| chat)
        .collect()
}

pub(crate) fn tab_chats(layout: &Layout) -> BTreeSet<String> {
    let chats = tab_panes(layout).into_iter().filter_map(chat_of);
    chats.map(|(_, chat)| chat).collect()
}

pub(crate) fn panes_showing(
    layout: &Layout,
    chats: &[String],
) -> Vec<PaneId> {
    chat_panes(layout)
        .into_iter()
        .filter(|(_, chat)| chats.contains(chat))
        .map(|(pane, _)| pane)
        .collect()
}

fn chat_panes(layout: &Layout) -> Vec<(PaneId, String)> {
    tab_panes(layout)
        .into_iter()
        .filter(|pane| pane.type_id.is_chat())
        .filter_map(chat_of)
        .collect()
}

fn tab_panes(layout: &Layout) -> Vec<PaneInstance> {
    layout
        .tabs()
        .iter()
        .flat_map(|tab| {
            tab.pane_ids()
                .into_iter()
                .filter_map(|pane| tab.pane(&pane).cloned())
        })
        .collect()
}

fn chat_of(pane: PaneInstance) -> Option<(PaneId, String)> {
    let chat = pane.params.get(CHAT_ID)?.to_owned();
    Some((pane.pane_id, chat))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::panes::kinds::core_panes::{CHAT, EDITOR, TERMINAL};
    use crate::test_support::{add_tab, chat_pane, one_tab, open_in, pane};

    #[test]
    fn every_chat_some_pane_shows_is_open_whatever_its_tab() {
        let mut layout = Layout::default();
        assert!(open_chats(&layout).is_empty(), "nothing open");
        layout = one_tab([
            chat_pane(("c1", CHAT), "t-1"),
            pane("x", TERMINAL),
            chat_pane(("f", EDITOR), "t-3"),
        ]);
        add_tab(&mut layout, 2);
        open_in(&mut layout, 2, chat_pane(("c2", CHAT), "t-2"));
        assert_eq!(
            open_chats(&layout),
            ["t-1", "t-2"],
            "both chats, no terminal"
        );
        let shown: Vec<String> = tab_chats(&layout).into_iter().collect();
        assert_eq!(shown, ["t-1", "t-2", "t-3"], "and the chat of a file tab");
        let held = ["t-2".to_owned(), "x".to_owned()];
        assert_eq!(
            panes_showing(&layout, &held),
            [PaneId::new("c2").unwrap()],
            "the panes of the named chats"
        );
    }
}
