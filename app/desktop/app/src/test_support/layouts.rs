use layout::{Command, Layout, PaneInstance, TabId};
use protocol::{PaneId, PaneState, PaneTypeId, Params};

use crate::panes::kinds::core_panes::TERMINAL;

pub(crate) fn pane(
    id: &str,
    type_id: &str,
) -> PaneInstance {
    PaneInstance {
        pane_id: PaneId::new(id).unwrap(),
        type_id: PaneTypeId::new(type_id).unwrap(),
        params: Params::default(),
        state: PaneState::default(),
    }
}

pub(crate) fn chat_pane(
    (id, type_id): (&str, &str),
    chat: &str,
) -> PaneInstance {
    PaneInstance {
        params: Params::default().with("threadId", chat).unwrap(),
        ..pane(id, type_id)
    }
}

pub(crate) fn add_tab(
    layout: &mut Layout,
    tab: u64,
) {
    let add = Command::AddTab {
        id: TabId(tab),
        title: String::new(),
        space: layout::SpaceId::default(),
    };
    layout.apply(add).unwrap();
}

pub(crate) fn open_in(
    layout: &mut Layout,
    tab: u64,
    instance: PaneInstance,
) {
    let open = Command::OpenPane {
        tab: TabId(tab),
        instance,
        placement: None,
    };
    layout.apply(open).unwrap();
}

pub(crate) fn one_tab(panes: impl IntoIterator<Item = PaneInstance>) -> Layout {
    let mut layout = Layout::default();
    add_tab(&mut layout, 1);
    for instance in panes {
        open_in(&mut layout, 1, instance);
    }
    layout
}

pub(crate) fn terminal_layout(chat: Option<&str>) -> Layout {
    let terminal = match chat {
        Some(chat) => chat_pane(("t1", TERMINAL), chat),
        None => pane("t1", TERMINAL),
    };
    one_tab([terminal])
}
