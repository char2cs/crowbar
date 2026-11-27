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
    use layout::{Command, PaneInstance, TabId};

    use super::*;
    use crate::panes::kinds::core_panes::{chat_params, chat_type};
    use crate::panes::kinds::pane_spec::PaneSpec;

    fn open(
        layout: &mut Layout,
        instance: PaneInstance,
    ) {
        layout
            .apply(Command::OpenPane {
                tab: TabId(1),
                instance,
                placement: None,
            })
            .unwrap();
    }

    #[test]
    fn only_review_panes_are_found_each_with_its_chat() {
        let mut layout = Layout::default();
        let title = String::new();
        layout
            .apply(Command::AddTab {
                id: TabId(1),
                title,
                space: layout::SpaceId::default(),
            })
            .unwrap();
        let review = |chat: &str| PaneSpec::review(chat, None).unwrap();
        open(
            &mut layout,
            review("c1").instance(PaneId::new("r1").unwrap()),
        );
        open(
            &mut layout,
            review("c2").instance(PaneId::new("r2").unwrap()),
        );
        let chat = PaneInstance {
            pane_id: PaneId::new("ch").unwrap(),
            type_id: chat_type().unwrap(),
            params: chat_params("c1").unwrap(),
            state: protocol::PaneState::default(),
        };
        open(&mut layout, chat);
        let found = review_panes(&layout);
        let expected = [
            (PaneId::new("r1").unwrap(), "c1".to_owned()),
            (PaneId::new("r2").unwrap(), "c2".to_owned()),
        ];
        assert_eq!(found, expected);
        assert_eq!(review_chat(&layout, &PaneId::new("ch").unwrap()), None);
    }
}
