use protocol::{PaneId, PaneState, PaneTypeId, Params};

use crate::layout::tab::Tab;
use crate::layout::tab::pane_instance::PaneInstance;
use crate::tab_id::TabId;
use crate::tree::edge::Edge;
use crate::tree::geometry::rect::Rect;
use crate::tree::placement::Placement;

pub(crate) fn id(name: &str) -> PaneId {
    PaneId::new(name).unwrap()
}

pub(crate) fn pane(name: &str) -> PaneInstance {
    PaneInstance {
        pane_id: id(name),
        type_id: PaneTypeId::new("core.chat").unwrap(),
        params: Params::default(),
        state: PaneState::default(),
    }
}

pub(crate) fn editor(name: &str) -> PaneInstance {
    PaneInstance {
        type_id: PaneTypeId::new("core.terminal").unwrap(),
        params: Params::default().with("terminalId", name).unwrap(),
        ..pane(name)
    }
}

pub(crate) fn at(
    target: &str,
    edge: Edge,
) -> Placement {
    Placement {
        target: id(target),
        edge,
    }
}

pub(crate) fn order(tab: &Tab) -> Vec<String> {
    tab.pane_ids().iter().map(ToString::to_string).collect()
}

pub(crate) fn tab_with(names: &[&str]) -> Tab {
    let mut tab = Tab::new(TabId(1), "t", crate::space_id::SpaceId::default());
    for name in names {
        tab.open(pane(name), None).unwrap();
    }
    tab
}

pub(crate) fn screen(
    width: f32,
    height: f32,
) -> Rect {
    Rect {
        x: 0.0,
        y: 0.0,
        width,
        height,
    }
}
