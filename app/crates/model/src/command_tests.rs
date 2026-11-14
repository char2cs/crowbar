use super::*;

#[test]
fn only_arrangement_changes_are_structural() {
    let pane = PaneId::new("a").unwrap();
    let tab = TabId(1);
    let ratio_change = Command::ResizeSplit {
        tab,
        path: Vec::new(),
        ratio: 0.5,
    };
    let state_change = Command::SetPaneState {
        pane: pane.clone(),
        state: PaneState::default(),
    };
    let structural = [
        Command::CloseTab { tab },
        Command::MoveTab { tab, to_index: 0 },
        Command::ClosePane { pane: pane.clone() },
    ];
    let view_only = [
        Command::ActivateTab { tab },
        Command::FocusPane { pane },
        ratio_change,
        state_change,
    ];
    for command in structural {
        assert!(command.is_structural(), "{command:?}");
    }
    for command in view_only {
        assert!(!command.is_structural(), "{command:?}");
    }
}
