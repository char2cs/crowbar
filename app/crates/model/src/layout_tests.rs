use protocol::{PaneState, PaneTypeId, Params};

use super::*;
use crate::edge::Edge;

fn id(name: &str) -> PaneId {
    PaneId::new(name).unwrap()
}

fn instance(name: &str) -> PaneInstance {
    PaneInstance {
        pane_id: id(name),
        type_id: PaneTypeId::new("core.chat").unwrap(),
        params: Params::default(),
        state: PaneState::default(),
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

fn add(tab: u64) -> Command {
    Command::AddTab {
        id: TabId(tab),
        title: format!("tab {tab}"),
    }
}

#[test]
fn adding_a_tab_activates_it() {
    let layout = layout(vec![add(1), add(2)]);
    assert_eq!(layout.active_tab().map(Tab::id), Some(TabId(2)), "active");
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
fn closing_the_active_tab_activates_its_neighbour() {
    let mut layout = layout(vec![
        add(1),
        add(2),
        add(3),
        Command::ActivateTab { tab: TabId(2) },
    ]);
    layout.apply(Command::CloseTab { tab: TabId(2) }).unwrap();
    assert_eq!(layout.active_tab().map(Tab::id), Some(TabId(3)), "next");
    layout.apply(Command::CloseTab { tab: TabId(3) }).unwrap();
    assert_eq!(layout.active_tab().map(Tab::id), Some(TabId(1)), "previous");
    layout.apply(Command::CloseTab { tab: TabId(1) }).unwrap();
    assert!(
        layout.active_tab().is_none() && layout.tabs().is_empty(),
        "none left"
    );
    assert!(layout.validate().is_ok(), "valid");
}

#[test]
fn closing_an_inactive_tab_keeps_the_active_one() {
    let mut layout = layout(vec![add(1), add(2)]);
    layout.apply(Command::CloseTab { tab: TabId(1) }).unwrap();
    assert_eq!(
        layout.active_tab().map(Tab::id),
        Some(TabId(2)),
        "active kept"
    );
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
}

#[test]
fn pane_ids_are_unique_across_tabs() {
    let mut layout = layout(vec![add(1), add(2), open(1, "a")]);
    assert_eq!(
        layout.apply(open(2, "a")),
        Err(Error::DuplicatePane(id("a"))),
        "duplicate"
    );
    assert_eq!(
        layout.apply(open(9, "b")),
        Err(Error::TabNotFound(TabId(9))),
        "no tab"
    );
}

#[test]
fn opening_a_pane_activates_its_tab() {
    let layout = layout(vec![
        add(1),
        add(2),
        Command::ActivateTab { tab: TabId(1) },
        open(2, "a"),
    ]);
    assert_eq!(layout.active_tab().map(Tab::id), Some(TabId(2)), "active");
    assert_eq!(
        layout.tab_of(&id("a")).map(Tab::id),
        Some(TabId(2)),
        "owner"
    );
}

#[test]
fn focusing_a_pane_activates_its_tab() {
    let mut layout = layout(vec![add(1), open(1, "a"), add(2)]);
    layout.apply(Command::FocusPane { pane: id("a") }).unwrap();
    assert_eq!(
        layout.active_tab().map(Tab::id),
        Some(TabId(1)),
        "tab 1 active"
    );
}

#[test]
fn moving_a_pane_to_another_tab_keeps_id_and_state() {
    let state = PaneState::new(serde_json::json!({"k": 1})).unwrap();
    let mut layout = layout(vec![
        add(1),
        add(2),
        open(1, "a"),
        open(1, "b"),
        Command::SetPaneState {
            pane: id("a"),
            state: state.clone(),
        },
    ]);
    layout
        .apply(Command::MovePaneToTab {
            pane: id("a"),
            to: TabId(2),
            placement: None,
        })
        .unwrap();
    layout.validate().unwrap();
    let tab = layout.tab_of(&id("a")).unwrap();
    assert_eq!(tab.id(), TabId(2), "moved");
    assert_eq!(
        tab.pane(&id("a")).map(|p| p.state.clone()),
        Some(state),
        "state kept"
    );
    assert_eq!(
        layout.tab(TabId(1)).map(|t| t.pane_ids().len()),
        Some(1),
        "source shrank"
    );
    assert_eq!(
        layout.active_tab().map(Tab::id),
        Some(TabId(2)),
        "destination active"
    );
}

#[test]
fn a_failed_move_leaves_everything_as_it_was() {
    let mut layout = layout(vec![add(1), add(2), open(1, "a"), open(2, "b")]);
    let before = layout.clone();
    let bad = Some(Placement {
        target: id("zz"),
        edge: Edge::Left,
    });
    assert!(
        layout
            .apply(Command::MovePaneToTab {
                pane: id("a"),
                to: TabId(2),
                placement: bad
            })
            .is_err(),
        "bad target"
    );
    assert_eq!(layout, before, "atomic: the pane is not lost from tab 1");
    assert_eq!(
        layout.apply(Command::MovePaneToTab {
            pane: id("a"),
            to: TabId(1),
            placement: None
        }),
        Err(Error::SamePane(id("a"))),
        "same tab"
    );
    assert_eq!(layout, before, "unchanged");
}

#[test]
fn closing_a_tab_drops_its_panes() {
    let mut layout = layout(vec![add(1), open(1, "a")]);
    layout.apply(Command::CloseTab { tab: TabId(1) }).unwrap();
    assert!(layout.tab_of(&id("a")).is_none(), "pane gone with its tab");
}

#[test]
fn pane_commands_find_the_owning_tab() {
    let mut layout = layout(vec![
        add(1),
        add(2),
        open(1, "a"),
        open(2, "b"),
        open(2, "c"),
    ]);
    layout
        .apply(Command::SwapPanes {
            a: id("b"),
            b: id("c"),
        })
        .unwrap();
    layout.apply(Command::ClosePane { pane: id("c") }).unwrap();
    layout
        .apply(Command::ResizeSplit {
            tab: TabId(1),
            path: vec![],
            ratio: 0.3,
        })
        .unwrap_err();
    assert_eq!(
        layout.apply(Command::ClosePane { pane: id("c") }),
        Err(Error::PaneNotFound(id("c"))),
        "gone"
    );
    layout.validate().unwrap();
}

#[test]
fn serde_round_trips_and_validates_on_load() {
    let layout = layout(vec![add(1), open(1, "a"), open(1, "b")]);
    let json = serde_json::to_string(&layout).unwrap();
    assert_eq!(
        serde_json::from_str::<Layout>(&json).unwrap(),
        layout,
        "round trip"
    );
    let broken = json.replace(r#""active":1"#, r#""active":7"#);
    assert!(
        serde_json::from_str::<Layout>(&broken).is_err(),
        "unknown active tab"
    );
    let orphan = json.replace(r#""pane":"b""#, r#""pane":"ghost""#);
    assert!(
        serde_json::from_str::<Layout>(&orphan).is_err(),
        "leaf without pane"
    );
}
