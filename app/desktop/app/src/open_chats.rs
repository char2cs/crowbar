use layout::Layout;

const CHAT_ID: &str = "threadId";

pub(crate) fn open_chats(layout: &Layout) -> Vec<String> {
    layout
        .tabs()
        .iter()
        .flat_map(|tab| {
            tab.pane_ids()
                .into_iter()
                .filter_map(|pane| tab.pane(&pane).cloned())
        })
        .filter(|pane| pane.type_id.is_chat())
        .filter_map(|pane| pane.params.get(CHAT_ID).map(str::to_owned))
        .collect()
}

#[cfg(test)]
mod tests {
    use layout::{Command, PaneInstance, TabId};
    use protocol::{PaneId, PaneState, PaneTypeId, Params};

    use super::*;
    use crate::core_panes::{chat_params, chat_type};

    fn pane(
        name: &str,
        (type_id, params): (PaneTypeId, Params),
    ) -> PaneInstance {
        PaneInstance {
            pane_id: PaneId::new(name).unwrap(),
            type_id,
            params,
            state: PaneState::default(),
        }
    }

    fn open(
        tab: u64,
        instance: PaneInstance,
    ) -> Command {
        Command::OpenPane {
            tab: TabId(tab),
            instance,
            placement: None,
        }
    }

    fn add(tab: u64) -> Command {
        Command::AddTab {
            id: TabId(tab),
            title: String::new(),
        }
    }

    #[test]
    fn every_chat_some_pane_shows_is_open_whatever_its_tab() {
        let terminal = PaneTypeId::new("core.terminal").unwrap();
        let commands = [
            add(1),
            open(
                1,
                pane("c1", (chat_type().unwrap(), chat_params("t-1").unwrap())),
            ),
            open(1, pane("x", (terminal, Params::default()))),
            add(2),
            open(
                2,
                pane("c2", (chat_type().unwrap(), chat_params("t-2").unwrap())),
            ),
        ];
        let mut layout = Layout::default();
        assert!(open_chats(&layout).is_empty(), "nothing open");
        for command in commands {
            layout.apply(command).unwrap();
        }
        assert_eq!(
            open_chats(&layout),
            ["t-1", "t-2"],
            "both chats, no terminal"
        );
    }
}
