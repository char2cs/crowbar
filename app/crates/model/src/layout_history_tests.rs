use protocol::{PaneId, PaneState, PaneTypeId, Params};
use serde_json::json;

use super::*;
use crate::pane_instance::PaneInstance;
use crate::tab_id::TabId;

fn id(name: &str) -> PaneId {
    PaneId::new(name).unwrap()
}

fn open(name: &str) -> Command {
    Command::OpenPane {
        tab: TabId(1),
        instance: PaneInstance {
            pane_id: id(name),
            type_id: PaneTypeId::new("core.chat").unwrap(),
            params: Params::default(),
            state: PaneState::default(),
        },
        placement: None,
    }
}

fn history() -> LayoutHistory {
    let mut history = LayoutHistory::default();
    history
        .apply(Command::AddTab {
            id: TabId(1),
            title: "t".into(),
        })
        .unwrap();
    history.apply(open("a")).unwrap();
    history.apply(open("b")).unwrap();
    history
}

#[test]
fn undo_and_redo_walk_structural_changes() {
    let mut history = history();
    let full = history.layout().clone();
    assert!(history.undo(), "undo open b");
    assert_eq!(
        history.layout().tab(TabId(1)).map(|t| t.pane_ids().len()),
        Some(1),
        "one pane"
    );
    assert!(history.can_redo(), "redo available");
    assert!(history.redo(), "redo");
    assert_eq!(history.layout(), &full, "back to full");
    assert!(!history.redo(), "nothing more to redo");
}

#[test]
fn undo_runs_out() {
    let mut history = history();
    assert!(
        history.undo() && history.undo() && history.undo(),
        "three commands"
    );
    assert!(!history.can_undo() && !history.undo(), "exhausted");
    assert!(history.layout().tabs().is_empty(), "back to empty");
}

#[test]
fn non_structural_commands_are_not_undoable_and_keep_redo() {
    let mut history = history();
    history.undo();
    history
        .apply(Command::ResizeSplit {
            tab: TabId(1),
            path: vec![],
            ratio: 0.5,
        })
        .unwrap_err();
    history.apply(Command::FocusPane { pane: id("a") }).unwrap();
    assert!(history.can_redo(), "focus does not clear redo");
    let depth = history.undo.len();
    history.apply(Command::FocusPane { pane: id("a") }).unwrap();
    assert_eq!(history.undo.len(), depth, "focus is not recorded");
}

#[test]
fn a_new_structural_command_clears_redo() {
    let mut history = history();
    history.undo();
    history.apply(open("c")).unwrap();
    assert!(!history.can_redo(), "redo cleared");
}

#[test]
fn refused_commands_leave_history_untouched() {
    let mut history = history();
    let before = history.clone();
    assert!(
        history.apply(Command::CloseTab { tab: TabId(9) }).is_err(),
        "unknown tab"
    );
    assert_eq!(history, before, "unchanged");
}

#[test]
fn undoing_a_close_keeps_the_latest_state_of_other_panes() {
    let mut history = history();
    history.apply(Command::ClosePane { pane: id("b") }).unwrap();
    let state = PaneState::new(json!({"draft": "hello"})).unwrap();
    history
        .apply(Command::SetPaneState {
            pane: id("a"),
            state: state.clone(),
        })
        .unwrap();
    assert!(history.undo(), "restore b");
    let tab = history.layout().tab(TabId(1)).unwrap();
    assert!(tab.pane(&id("b")).is_some(), "b is back");
    assert_eq!(
        tab.pane(&id("a")).map(|p| p.state.clone()),
        Some(state),
        "a keeps its newer state"
    );
    assert!(history.layout().validate().is_ok(), "valid");
}

#[test]
fn history_is_bounded() {
    let mut history = LayoutHistory::default();
    for n in 0..(MAX_UNDO as u64 + 10) {
        history
            .apply(Command::AddTab {
                id: TabId(n),
                title: String::new(),
            })
            .unwrap();
    }
    assert_eq!(history.undo.len(), MAX_UNDO, "oldest snapshots dropped");
}
