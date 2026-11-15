use layout_view::SidebarEdge;
use model::{Command, Layout, PaneInstance, TabId};
use protocol::{PaneId, PaneState, PaneTypeId, Params};

use super::*;

fn one_pane_layout() -> Layout {
    let mut layout = Layout::default();
    layout
        .apply(Command::AddTab {
            id: TabId(1),
            title: "t".into(),
        })
        .unwrap();
    layout
        .apply(Command::OpenPane {
            tab: TabId(1),
            instance: PaneInstance {
                pane_id: PaneId::new("p1").unwrap(),
                type_id: PaneTypeId::new("core.chat").unwrap(),
                params: Params::default(),
                state: PaneState::default(),
            },
            placement: None,
        })
        .unwrap();
    layout
}

#[test]
fn holes_sit_inside_the_area_in_window_coordinates() {
    let layout = one_pane_layout();
    let area = Bounds {
        x: 294.0,
        y: 42.0,
        width: 800.0,
        height: 600.0,
    };
    let chrome = ChromeContext {
        sidebar: SidebarEdge::Left,
        tab_strip: true,
    };
    let panes = visible_panes(layout.active_tab(), area, chrome);
    assert_eq!(panes.len(), 1, "one pane");
    let bounds = panes[0].bounds;
    assert!(
        bounds.x >= area.x && bounds.y >= area.y,
        "starts inside the area: {bounds:?}"
    );
    assert!(
        bounds.x + bounds.width <= area.x + area.width
            && bounds.y + bounds.height <= area.y + area.height,
        "ends inside the area: {bounds:?}"
    );
}

#[test]
fn no_tab_means_no_panes() {
    let area = Bounds {
        x: 0.0,
        y: 0.0,
        width: 10.0,
        height: 10.0,
    };
    assert!(
        visible_panes(None, area, ChromeContext::default()).is_empty(),
        "none"
    );
}
