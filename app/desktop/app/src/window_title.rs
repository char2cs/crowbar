use layout::Layout;

use crate::core_panes::CHAT;
use crate::titles::Titles;
use crate::window_memory::WINDOW_TITLE;

const SEPARATOR: &str = " \u{2014} ";

pub(crate) fn window_title(
    layout: &Layout,
    titles: &Titles,
) -> String {
    let chat = focused_chat_title(layout, titles).filter(|title| !title.trim().is_empty());
    let Some(title) = chat else {
        return WINDOW_TITLE.to_owned();
    };
    format!("{title}{SEPARATOR}{WINDOW_TITLE}")
}

fn is_chat_pane(
    layout: &Layout,
    pane: &protocol::PaneId,
) -> bool {
    layout
        .pane(pane)
        .is_some_and(|found| found.type_id.as_str() == CHAT)
}

fn focused_chat_title(
    layout: &Layout,
    titles: &Titles,
) -> Option<String> {
    let tab = layout.active_tab()?;
    let focused = tab.focused()?;
    let is_chat = |pane: &protocol::PaneId| is_chat_pane(layout, pane);
    let chat = Some(focused)
        .filter(|pane| is_chat(pane))
        .cloned()
        .or_else(|| tab.pane_ids().into_iter().find(is_chat))?;
    titles.get(&chat).map(|title| title.to_string())
}

#[cfg(test)]
mod tests {
    use layout::{Command, PaneInstance, TabId};
    use protocol::{PaneId, PaneState, PaneTitle, PaneTypeId, Params};

    use super::*;
    use crate::core_panes::{EDITOR, chat_params, chat_type};

    fn pane_of(
        id: &str,
        (type_id, params): (PaneTypeId, Params),
    ) -> Command {
        let instance = PaneInstance {
            pane_id: PaneId::new(id).unwrap(),
            type_id,
            params,
            state: PaneState::default(),
        };
        Command::OpenPane {
            tab: TabId(1),
            instance,
            placement: None,
        }
    }

    fn chat(id: &str) -> Command {
        pane_of(id, (chat_type().unwrap(), chat_params(id).unwrap()))
    }

    fn editor(id: &str) -> Command {
        pane_of(id, (PaneTypeId::new(EDITOR).unwrap(), Params::default()))
    }

    fn layout_of(panes: Vec<Command>) -> Layout {
        let mut layout = Layout::default();
        let add = Command::AddTab {
            id: TabId(1),
            title: String::new(),
        };
        layout.apply(add).unwrap();
        for open in panes {
            layout.apply(open).unwrap();
        }
        layout
    }

    fn titled(entries: &[(&str, &str)]) -> Titles {
        let titles = Titles::default();
        for (pane, title) in entries {
            titles.set(PaneId::new(*pane).unwrap(), PaneTitle::new(*title).unwrap());
        }
        titles
    }

    #[test]
    fn the_title_names_the_focused_chat_and_falls_back_to_the_product() {
        let cases = [
            (
                "a titled chat",
                layout_of(vec![chat("c")]),
                titled(&[("c", "Fix the build")]),
                "Fix the build \u{2014} Crowbar",
            ),
            (
                "an editor focused names the chat beside it",
                layout_of(vec![chat("c"), editor("e")]),
                titled(&[("c", "Fix the build"), ("e", "a.rs")]),
                "Fix the build \u{2014} Crowbar",
            ),
            (
                "a chat without a title yet",
                layout_of(vec![chat("c")]),
                Titles::default(),
                "Crowbar",
            ),
            (
                "no chat in the tab",
                layout_of(vec![editor("e")]),
                titled(&[("e", "a.rs")]),
                "Crowbar",
            ),
            ("no tab", Layout::default(), Titles::default(), "Crowbar"),
        ];
        for (name, layout, titles, expected) in cases {
            assert_eq!(window_title(&layout, &titles), expected, "{name}");
        }
    }
}
