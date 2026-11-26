//! The review panes a viewed-marks change reaches: each review pane knows its chat.

use layout::Layout;
use protocol::PaneId;

use crate::panes::kinds::core_panes::REVIEW;

pub(crate) type ViewedChanges = Vec<(String, Option<String>)>;

pub(crate) fn review_chat(
    layout: &Layout,
    pane: &PaneId,
) -> Option<String> {
    let instance = layout
        .pane(pane)
        .filter(|instance| instance.type_id.as_str() == REVIEW)?;
    instance.params.get("threadId").map(str::to_owned)
}

pub(crate) fn review_panes(layout: &Layout) -> Vec<(PaneId, String)> {
    layout
        .tabs()
        .iter()
        .flat_map(layout::Tab::pane_ids)
        .filter_map(|pane| review_chat(layout, &pane).map(|chat| (pane, chat)))
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::panes::kinds::core_panes::CHAT;
    use crate::panes::kinds::pane_spec::PaneSpec;
    use crate::test_support::{chat_pane, one_tab};

    #[test]
    fn only_review_panes_are_found_each_with_its_chat() {
        let review = |(chat, pane): (&str, &str)| {
            let spec = PaneSpec::review(chat, None).unwrap();
            spec.instance(PaneId::new(pane).unwrap())
        };
        let layout = one_tab([
            review(("c1", "r1")),
            review(("c2", "r2")),
            chat_pane(("ch", CHAT), "c1"),
        ]);
        let found = review_panes(&layout);
        let expected = [
            (PaneId::new("r1").unwrap(), "c1".to_owned()),
            (PaneId::new("r2").unwrap(), "c2".to_owned()),
        ];
        assert_eq!(found, expected);
        assert_eq!(review_chat(&layout, &PaneId::new("ch").unwrap()), None);
    }
}
