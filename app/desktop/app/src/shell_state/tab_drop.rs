use layout::{Command, Layout, Placement, TabId};

use super::ShellState;

impl ShellState {
    pub(crate) fn drop_tab(
        &mut self,
        tab: TabId,
        (onto, placement): (TabId, Placement),
    ) {
        let commands = moves_of_chats(self.history.layout(), tab, (onto, &placement));
        self.apply(commands);
    }
}

fn moves_of_chats(
    layout: &Layout,
    tab: TabId,
    (onto, placement): (TabId, &Placement),
) -> Vec<Command> {
    let Some(dragged) = layout.tab(tab).filter(|_| tab != onto) else {
        return Vec::new();
    };
    dragged
        .pane_ids()
        .into_iter()
        .filter(|pane| {
            dragged
                .pane(pane)
                .is_some_and(|found| found.type_id.is_chat())
        })
        .map(|pane| Command::MovePaneToTab {
            pane,
            to: onto,
            placement: Some(placement.clone()),
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use layout::Edge;
    use protocol::PaneId;

    use super::*;
    use crate::core_panes::{chat_params, chat_type};
    use crate::test_support::shell_rig;

    fn chat_panes(shell: &ShellState) -> Vec<Vec<PaneId>> {
        shell
            .layout()
            .tabs()
            .iter()
            .map(layout::Tab::pane_ids)
            .collect()
    }

    fn dropped_beside_the_first_chat(edge: Edge) -> (Vec<Vec<PaneId>>, PaneId) {
        let mut rig = shell_rig();
        for chat in ["t1", "t2"] {
            rig.shell
                .open_view(chat_type().unwrap(), chat_params(chat).unwrap());
        }
        let tabs: Vec<TabId> = rig
            .shell
            .layout()
            .tabs()
            .iter()
            .map(layout::Tab::id)
            .collect();
        let target = chat_panes(&rig.shell)[0][0].clone();
        let placement = Placement {
            target: target.clone(),
            edge,
        };
        rig.shell.drop_tab(tabs[1], (tabs[0], placement));
        (chat_panes(&rig.shell), target)
    }

    #[test]
    fn a_view_tab_dropped_beside_a_pane_brings_its_chat_to_that_side() {
        for (edge, target_at) in [(Edge::Right, 0), (Edge::Left, 1)] {
            let (views, target) = dropped_beside_the_first_chat(edge);
            assert_eq!(views.len(), 1, "{edge:?}: one view left: {views:?}");
            assert_eq!(views[0].len(), 2, "{edge:?}: both chats in it");
            assert_eq!(views[0][target_at], target, "{edge:?}: {views:?}");
        }
    }

    #[test]
    fn a_view_tab_dropped_on_itself_moves_nothing() {
        let mut rig = shell_rig();
        rig.shell
            .open_view(chat_type().unwrap(), chat_params("t1").unwrap());
        let tab = rig.shell.layout().tabs()[0].id();
        let target = chat_panes(&rig.shell)[0][0].clone();
        let placement = Placement {
            target,
            edge: Edge::Right,
        };
        let before = chat_panes(&rig.shell);
        rig.shell.drop_tab(tab, (tab, placement));
        assert_eq!(chat_panes(&rig.shell), before);
    }
}
