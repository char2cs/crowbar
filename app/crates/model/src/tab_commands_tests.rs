use protocol::{PaneState, PaneTypeId, Params};

use crate::command::Command;
use crate::error::Error;
use crate::layout::{Layout, MAX_TABS};
use crate::pane_instance::PaneInstance;
use crate::rect::Rect;
use crate::tab::Tab;
use crate::tab_id::TabId;

fn instance(name: &str) -> PaneInstance {
    PaneInstance {
        pane_id: protocol::PaneId::new(name).unwrap(),
        type_id: PaneTypeId::new("core.chat").unwrap(),
        params: Params::default(),
        state: PaneState::default(),
    }
}

fn add(tab: u64) -> Command {
    Command::AddTab {
        id: TabId(tab),
        title: format!("tab {tab}"),
    }
}

fn open(tab: u64, name: &str) -> Command {
    Command::OpenPane {
        tab: TabId(tab),
        instance: instance(name),
        placement: None,
    }
}

fn layout(commands: Vec<Command>) -> Layout {
    let mut layout = Layout::default();
    for command in commands {
        layout.apply(command).unwrap();
    }
    layout.validate().unwrap();
    layout
}

fn active(layout: &Layout) -> Option<TabId> {
    layout.active_tab().map(Tab::id)
}

#[test]
fn adding_a_tab_activates_it() {
    let layout = layout(vec![add(1), add(2)]);
    assert_eq!(active(&layout), Some(TabId(2)), "active");
    assert_eq!(layout.tabs().len(), 2, "count");
}

#[test]
fn duplicate_tab_ids_are_refused() {
    let mut layout = layout(vec![add(1)]);
    assert_eq!(
        layout.apply(add(1)),
        Err(Error::DuplicateTab(TabId(1))),
        "duplicate"
    );
}

#[test]
fn the_number_of_tabs_is_bounded() {
    let mut layout = Layout::default();
    for tab in 0..MAX_TABS as u64 {
        layout.apply(add(tab)).unwrap();
    }
    let before = layout.clone();
    assert_eq!(
        layout.apply(add(u64::MAX)),
        Err(Error::TooManyTabs { limit: MAX_TABS }),
        "full"
    );
    assert_eq!(layout, before, "unchanged");
}

#[test]
fn closing_the_active_tab_activates_its_neighbour() {
    let mut layout = layout(vec![
        add(1),
        add(2),
        add(3),
        Command::ActivateTab { tab: TabId(2) },
    ]);
    layout.apply(Command::CloseTab { tab: TabId(2) }).unwrap();
    assert_eq!(active(&layout), Some(TabId(3)), "next");
    layout.apply(Command::CloseTab { tab: TabId(3) }).unwrap();
    assert_eq!(active(&layout), Some(TabId(1)), "previous");
    layout.apply(Command::CloseTab { tab: TabId(1) }).unwrap();
    assert!(
        active(&layout).is_none() && layout.tabs().is_empty(),
        "none left"
    );
    layout.validate().unwrap();
}

#[test]
fn closing_an_inactive_tab_keeps_the_active_one() {
    let mut layout = layout(vec![add(1), add(2)]);
    layout.apply(Command::CloseTab { tab: TabId(1) }).unwrap();
    assert_eq!(active(&layout), Some(TabId(2)), "active kept");
}

#[test]
fn closing_a_tab_removes_its_panes_and_reports_them() {
    let mut layout = layout(vec![add(1), open(1, "a"), open(1, "b"), add(2)]);
    let removed = layout.execute(Command::CloseTab { tab: TabId(1) }).unwrap();
    let mut ids: Vec<String> = removed.iter().map(|p| p.pane_id.to_string()).collect();
    ids.sort();
    assert_eq!(ids, ["a", "b"], "both panes handed back for undo");
    assert_eq!(layout.pane_count(), 0, "gone from the layout");
    assert_eq!(
        layout.apply(Command::CloseTab { tab: TabId(1) }),
        Err(Error::TabNotFound(TabId(1))),
        "again"
    );
}

#[test]
fn activating_needs_an_existing_tab() {
    let mut layout = layout(vec![add(1), add(2)]);
    layout
        .apply(Command::ActivateTab { tab: TabId(1) })
        .unwrap();
    assert_eq!(active(&layout), Some(TabId(1)), "switched");
    assert_eq!(
        layout.apply(Command::ActivateTab { tab: TabId(9) }),
        Err(Error::TabNotFound(TabId(9))),
        "missing"
    );
    assert_eq!(active(&layout), Some(TabId(1)), "unchanged");
}

#[test]
fn tabs_reorder_within_range() {
    let mut layout = layout(vec![add(1), add(2), add(3)]);
    layout
        .apply(Command::MoveTab {
            tab: TabId(3),
            to_index: 0,
        })
        .unwrap();
    let ids: Vec<u64> = layout.tabs().iter().map(|t| t.id().0).collect();
    assert_eq!(ids, [3, 1, 2], "moved to front");
    assert_eq!(
        layout.apply(Command::MoveTab {
            tab: TabId(3),
            to_index: 3
        }),
        Err(Error::IndexOutOfRange { index: 3, len: 3 }),
        "out of range"
    );
    assert_eq!(
        layout.apply(Command::MoveTab {
            tab: TabId(7),
            to_index: 0
        }),
        Err(Error::TabNotFound(TabId(7))),
        "missing"
    );
}

#[test]
fn rename_changes_only_the_title() {
    let mut layout = layout(vec![add(1)]);
    layout
        .apply(Command::RenameTab {
            tab: TabId(1),
            title: "new".into(),
        })
        .unwrap();
    assert_eq!(layout.tab(TabId(1)).map(Tab::title), Some("new"), "title");
    assert_eq!(
        layout.apply(Command::RenameTab {
            tab: TabId(2),
            title: "x".into()
        }),
        Err(Error::TabNotFound(TabId(2))),
        "missing"
    );
}

const BOUNDS: Rect = Rect {
    x: 0.0,
    y: 0.0,
    width: 1006.0,
    height: 400.0,
};

fn widths(layout: &Layout) -> Vec<f32> {
    layout.tab(TabId(1)).map_or_else(Vec::new, |tab| {
        tab.layout(BOUNDS).iter().map(|s| s.rect.width).collect()
    })
}

#[test]
fn resize_and_reset_address_a_split_by_path() {
    let mut layout = layout(vec![add(1), open(1, "a"), open(1, "b")]);
    let bounds = BOUNDS;
    layout
        .apply(Command::ResizeSplit {
            tab: TabId(1),
            path: Vec::new(),
            bounds,
            first_extent: 300.0,
        })
        .unwrap();
    assert_eq!(widths(&layout), [300.0, 700.0], "resized");
    layout
        .apply(Command::ResetSplit {
            tab: TabId(1),
            path: Vec::new(),
        })
        .unwrap();
    assert_eq!(widths(&layout), [500.0, 500.0], "reset");
}

#[test]
fn resize_and_reset_refuse_a_missing_tab_or_path() {
    let mut layout = layout(vec![add(1), open(1, "a"), open(1, "b")]);
    let refused = Command::ResizeSplit {
        tab: TabId(5),
        path: Vec::new(),
        bounds: BOUNDS,
        first_extent: 1.0,
    };
    assert_eq!(
        layout.apply(refused),
        Err(Error::TabNotFound(TabId(5))),
        "tab"
    );
    let refused = Command::ResetSplit {
        tab: TabId(1),
        path: vec![crate::branch::Branch::First],
    };
    assert_eq!(layout.apply(refused), Err(Error::NoSuchSplit), "path");
}
