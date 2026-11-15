use model::{Command, PaneInstance, TabId};
use protocol::{PaneState, PaneTypeId, Params};
use webview::Bounds;

use super::*;

fn open(tab: u64, id: &str) -> Command {
    Command::OpenPane {
        tab: TabId(tab),
        instance: PaneInstance {
            pane_id: PaneId::new(id).unwrap(),
            type_id: PaneTypeId::new("core.chat").unwrap(),
            params: Params::default(),
            state: PaneState::default(),
        },
        placement: None,
    }
}

fn layout() -> Layout {
    let mut layout = Layout::default();
    for command in [
        Command::AddTab {
            id: TabId(1),
            title: "a".into(),
        },
        open(1, "p1"),
        open(1, "p2"),
        Command::AddTab {
            id: TabId(2),
            title: "b".into(),
        },
        open(2, "p3"),
        Command::ActivateTab { tab: TabId(1) },
    ] {
        layout.apply(command).unwrap();
    }
    layout
}

#[test]
fn lists_every_pane_and_the_focus_of_the_active_tab() {
    let hole = VisiblePane {
        pane: PaneId::new("p1").unwrap(),
        bounds: Bounds {
            x: 1.0,
            y: 2.0,
            width: 3.0,
            height: 4.0,
        },
    };
    let snapshot = LayoutSnapshot::from_layout(&layout(), ParentWindow::X11(5), vec![hole.clone()]);
    assert_eq!(snapshot.panes.len(), 3, "all tabs");
    assert_eq!(snapshot.visible, [hole], "the holes the shell gave");
    assert_eq!(
        snapshot.focused.as_ref().map(PaneId::as_str),
        Some("p2"),
        "focus of the active tab"
    );
}

#[test]
fn an_empty_layout_has_nothing_to_show() {
    let snapshot = LayoutSnapshot::from_layout(&Layout::default(), ParentWindow::X11(1), Vec::new());
    assert!(
        snapshot.panes.is_empty() && snapshot.visible.is_empty() && snapshot.focused.is_none(),
        "empty"
    );
}
