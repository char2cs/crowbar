use model::Layout;
use protocol::PaneId;

use crate::core_panes::CHAT;

pub(crate) fn opens_files(
    layout: &Layout,
    pane: &PaneId,
) -> bool {
    layout
        .pane(pane)
        .is_some_and(|instance| instance.type_id.as_str() != CHAT)
}

#[cfg(test)]
mod tests {
    use model::{Command, PaneInstance, TabId};
    use protocol::{PaneState, PaneTypeId, Params};

    use super::*;
    use crate::core_panes::EDITOR;

    fn layout_with(
        pane: &PaneId,
        type_id: &str,
    ) -> Layout {
        let mut layout = Layout::default();
        let tab = TabId(1);
        let title = String::new();
        layout.apply(Command::AddTab { id: tab, title }).unwrap();
        let instance = PaneInstance {
            pane_id: pane.clone(),
            type_id: PaneTypeId::new(type_id).unwrap(),
            params: Params::default(),
            state: PaneState::default(),
        };
        let placement = None;
        layout
            .apply(Command::OpenPane {
                tab,
                instance,
                placement,
            })
            .unwrap();
        layout
    }

    #[test]
    fn the_shell_opens_drops_except_on_chats_and_unknown_panes() {
        let pane = PaneId::new("p1").unwrap();
        let stranger = PaneId::new("p2").unwrap();
        let cases = [
            (layout_with(&pane, EDITOR), &pane, true),
            (layout_with(&pane, CHAT), &pane, false),
            (layout_with(&pane, EDITOR), &stranger, false),
        ];
        for (layout, on, want) in cases {
            assert_eq!(opens_files(&layout, on), want);
        }
    }
}
