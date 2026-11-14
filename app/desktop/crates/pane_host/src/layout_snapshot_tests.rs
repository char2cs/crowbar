use model::{Command, PaneInstance, TabId};
use protocol::{PaneState, PaneTypeId, Params};

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

#[test]
fn lists_every_pane_but_only_the_active_tabs_rectangles() {
    let mut layout = Layout::default();
    for command in [
        Command::AddTab { id: TabId(1), title: "a".into() },
        open(1, "p1"),
        open(1, "p2"),
        Command::AddTab { id: TabId(2), title: "b".into() },
        open(2, "p3"),
    ] {
        layout.apply(command).unwrap();
    }
    layout.apply(Command::ActivateTab { tab: TabId(1) }).unwrap();
    let content = Bounds { x: 200.0, y: 40.0, width: 800.0, height: 600.0 };
    let snapshot = LayoutSnapshot::from_layout(&layout, ParentWindow::X11(5), content);
    assert_eq!(snapshot.panes.len(), 3, "all tabs");
    let visible: Vec<(&str, f64, f64)> = snapshot.visible.iter().map(|v| (v.pane.as_str(), v.bounds.x, v.bounds.width)).collect();
    assert_eq!(visible, [("p1", 200.0, 400.0), ("p2", 600.0, 400.0)], "active tab tiles the content area");
    assert_eq!(snapshot.focused.as_ref().map(PaneId::as_str), Some("p2"), "focus of the active tab");
}

#[test]
fn an_empty_layout_has_nothing_visible() {
    let content = Bounds { x: 0.0, y: 0.0, width: 10.0, height: 10.0 };
    let snapshot = LayoutSnapshot::from_layout(&Layout::default(), ParentWindow::X11(1), content);
    assert!(snapshot.panes.is_empty() && snapshot.visible.is_empty() && snapshot.focused.is_none(), "empty");
}
