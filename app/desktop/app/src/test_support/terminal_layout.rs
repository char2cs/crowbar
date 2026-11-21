use layout::{Command, Layout, PaneInstance, TabId};
use protocol::{PaneId, PaneState, PaneTypeId, Params};

use crate::core_panes::TERMINAL;

pub(crate) fn terminal_layout(chat: Option<&str>) -> Layout {
    let mut layout = Layout::default();
    let tab = TabId(1);
    let add = Command::AddTab {
        id: tab,
        title: String::new(),
    };
    layout.apply(add).unwrap();
    let params = chat.map_or_else(Params::default, |chat| {
        Params::default().with("threadId", chat).unwrap()
    });
    let instance = PaneInstance {
        pane_id: PaneId::new("t1").unwrap(),
        type_id: PaneTypeId::new(TERMINAL).unwrap(),
        params,
        state: PaneState::default(),
    };
    let placement = None;
    let open = Command::OpenPane {
        tab,
        instance,
        placement,
    };
    layout.apply(open).unwrap();
    layout
}
