use layout::{Command, Layout};

use crate::panes::tracking::open_chats::{open_chats, panes_showing};

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
    use layout::{PaneInstance, TabId};
    use protocol::{PaneId, PaneState};

    use super::*;
    use crate::panes::kinds::core_panes::{chat_params, chat_type};

    fn chat(
        tab: u64,
        (name, thread): (&str, &str),
    ) -> Command {
        let instance = PaneInstance {
            pane_id: PaneId::new(name).unwrap(),
            type_id: chat_type().unwrap(),
            params: chat_params(thread).unwrap(),
            state: PaneState::default(),
        };
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
            space: layout::SpaceId::default(),
        }
    }

    #[test]
    fn a_saved_draft_is_dropped_and_its_lone_tab_with_it() {
        let mut layout = Layout::default();
        let commands = [
            add(1),
            chat(1, ("a", "c1")),
            chat(1, ("b", "draft-u1")),
            add(2),
            chat(2, ("c", "draft-u2")),
        ];
        for command in commands {
            layout.apply(command).unwrap();
        }
        let restored = without_drafts(layout);
        assert_eq!(open_chats(&restored), ["c1"], "only the real chat");
        assert_eq!(restored.tabs().len(), 1, "the draft's own tab closed");
    }
}
