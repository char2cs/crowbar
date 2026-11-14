use std::collections::BTreeMap;

use protocol::{PaneState, PaneTypeId, Params};
use serde_json::json;

use super::*;
use crate::edge::Edge;
use crate::placement::Placement;

fn id(name: &str) -> PaneId {
    PaneId::new(name).unwrap()
}

fn instance(name: &str) -> PaneInstance {
    PaneInstance {
        pane_id: id(name),
        type_id: PaneTypeId::new("core.terminal").unwrap(),
        params: Params::default(),
        state: PaneState::default(),
    }
}

fn chat(name: &str, chat_id: &str) -> PaneInstance {
    PaneInstance {
        type_id: PaneTypeId::new("core.chat").unwrap(),
        params: Params::default().with("chatId", chat_id).unwrap(),
        ..instance(name)
    }
}

fn open(tab: u64, instance: PaneInstance) -> Command {
    Command::OpenPane {
        tab: TabId(tab),
        instance,
        placement: None,
    }
}

fn add(tab: u64) -> Command {
    Command::AddTab {
        id: TabId(tab),
        title: format!("tab {tab}"),
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

fn sample() -> Layout {
    layout(vec![
        add(1),
        open(1, chat("a", "c1")),
        open(1, instance("b")),
        add(2),
        open(2, instance("c")),
    ])
}

#[test]
fn queries_find_tabs_and_panes() {
    let layout = sample();
    assert_eq!(layout.pane_count(), 3, "count");
    assert_eq!(
        layout.tab_of(&id("c")).map(Tab::id),
        Some(TabId(2)),
        "tab of"
    );
    assert_eq!(
        layout.pane(&id("b")).map(|p| p.pane_id.clone()),
        Some(id("b")),
        "pane"
    );
    assert!(
        layout.pane(&id("zz")).is_none() && layout.tab(TabId(9)).is_none(),
        "missing"
    );
    let found = layout.find_pane(
        &PaneTypeId::new("core.chat").unwrap(),
        &Params::default().with("chatId", "c1").unwrap(),
    );
    assert_eq!(
        found.map(|p| p.pane_id.clone()),
        Some(id("a")),
        "by identity"
    );
    let other_type = layout.find_pane(
        &PaneTypeId::new("core.terminal").unwrap(),
        &Params::default().with("chatId", "c1").unwrap(),
    );
    assert!(other_type.is_none(), "type is part of the identity");
    let no_params = layout.find_pane(
        &PaneTypeId::new("core.terminal").unwrap(),
        &Params::default(),
    );
    assert!(no_params.is_none(), "empty parameters identify nothing");
}

#[test]
fn a_refused_command_changes_nothing() {
    let mut layout = sample();
    let before = layout.clone();
    let refused = [
        open(9, instance("n")),
        open(1, instance("a")),
        Command::CloseTab { tab: TabId(9) },
        Command::ClosePane { pane: id("zz") },
        Command::MovePane {
            pane: id("a"),
            placement: Placement {
                target: id("a"),
                edge: Edge::Left,
            },
        },
    ];
    for command in refused {
        assert!(layout.apply(command).is_err());
        assert_eq!(layout, before, "unchanged");
    }
}

#[test]
fn validate_names_broken_invariants() {
    let broken_active = Layout {
        active: Some(TabId(9)),
        ..sample()
    };
    assert!(broken_active.validate().is_err(), "active tab missing");
    let no_active = Layout {
        active: None,
        ..sample()
    };
    assert!(no_active.validate().is_err(), "tabs but no active");
    let mut duplicated = sample();
    duplicated.tabs.push(duplicated.tabs[0].clone());
    assert!(duplicated.validate().is_err(), "tab id and panes twice");
    let mut same_chat = sample();
    same_chat.tabs[1] = Tab::restored(
        crate::tab_shape::TabShape {
            id: TabId(2),
            title: "two".into(),
            tree: Some(crate::split_node::SplitNode::Leaf { pane: id("d") }),
            focused: Some(id("d")),
        },
        [(id("d"), chat("d", "c1"))].into_iter().collect(),
    );
    assert!(same_chat.validate().is_err(), "one chat open twice");
}

#[test]
fn a_layout_round_trips_through_json_with_its_version() {
    let layout = sample();
    let value = serde_json::to_value(&layout).unwrap();
    assert_eq!(value["version"], json!(SCHEMA_VERSION), "versioned");
    let back: Layout = serde_json::from_value(value).unwrap();
    assert_eq!(back, layout, "round trip");
}

#[test]
fn an_unversioned_layout_from_the_first_format_is_migrated() {
    let mut value = serde_json::to_value(sample()).unwrap();
    value.as_object_mut().unwrap().remove("version");
    let migrated: Layout = serde_json::from_value(value).unwrap();
    assert_eq!(migrated, sample(), "same layout");
}

#[test]
fn a_layout_from_a_newer_app_or_with_stray_data_is_refused() {
    let value = serde_json::to_value(sample()).unwrap();
    let mut newer = value.clone();
    newer["version"] = json!(SCHEMA_VERSION + 1);
    let error = serde_json::from_value::<Layout>(newer)
        .unwrap_err()
        .to_string();
    assert!(error.contains("newer"), "{error}");
    let mut stray = value.clone();
    stray["surprise"] = json!(true);
    assert!(
        serde_json::from_value::<Layout>(stray).is_err(),
        "unknown field"
    );
    let mut ghost = value;
    ghost["active"] = json!(77);
    let error = serde_json::from_value::<Layout>(ghost)
        .unwrap_err()
        .to_string();
    assert!(error.contains("invalid layout"), "{error}");
}

#[test]
fn swapping_in_an_arrangement_keeps_live_panes_and_parks_the_rest() {
    let mut layout = sample();
    let before = layout.arrangement();
    let removed = layout
        .execute(Command::ClosePane { pane: id("b") })
        .unwrap();
    let step = UndoStep {
        arrangement: before.clone(),
        parked: removed
            .into_iter()
            .map(|p| (p.pane_id.clone(), p))
            .collect(),
    };
    let after_close = layout.arrangement();
    let inverse = layout.swap_arrangement(step).unwrap();
    assert_eq!(layout.arrangement(), before, "arrangement back");
    assert_eq!(layout.pane_count(), 3, "pane back");
    assert!(
        inverse.parked.is_empty() && inverse.arrangement == after_close,
        "inverse: {inverse:?}"
    );
    layout.validate().unwrap();
}

#[test]
fn an_arrangement_naming_a_vanished_pane_is_refused_untouched() {
    let mut layout = sample();
    let before = layout.clone();
    let mut arrangement = layout.arrangement();
    arrangement.tabs[0].tree = Some(crate::split_node::SplitNode::Leaf { pane: id("ghost") });
    let step = UndoStep {
        arrangement,
        parked: BTreeMap::new(),
    };
    assert!(layout.swap_arrangement(step).is_err(), "refused");
    assert_eq!(layout, before, "layout untouched");
}
