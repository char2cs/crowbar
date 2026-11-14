use super::*;
use crate::edge::Edge;

fn instance(pane: &PaneId) -> PaneInstance {
    PaneInstance {
        pane_id: pane.clone(),
        type_id: protocol::PaneTypeId::new("core.chat").unwrap(),
        params: protocol::Params::default(),
        state: PaneState::default(),
    }
}

fn tab_commands() -> [Command; 4] {
    let tab = TabId(1);
    [
        Command::AddTab {
            id: tab,
            title: String::new(),
        },
        Command::CloseTab { tab },
        Command::MoveTab { tab, to_index: 0 },
        Command::RenameTab {
            tab,
            title: String::new(),
        },
    ]
}

fn pane_commands() -> [Command; 6] {
    let pane = PaneId::new("a").unwrap();
    let other = PaneId::new("b").unwrap();
    let tab = TabId(1);
    let placement = Placement {
        target: other.clone(),
        edge: Edge::Left,
    };
    let instance = instance(&pane);
    [
        Command::OpenPane {
            tab,
            instance,
            placement: None,
        },
        Command::ClosePane { pane: pane.clone() },
        Command::MovePane {
            pane: pane.clone(),
            placement,
        },
        Command::MovePaneToTab {
            pane: pane.clone(),
            to: tab,
            placement: None,
        },
        Command::DetachPane {
            pane: pane.clone(),
            new_tab: tab,
            title: String::new(),
        },
        Command::SwapPanes {
            a: pane.clone(),
            b: other,
        },
    ]
}

fn view_only() -> [Command; 5] {
    let pane = PaneId::new("a").unwrap();
    let tab = TabId(1);
    [
        Command::ActivateTab { tab },
        Command::FocusPane { pane: pane.clone() },
        Command::ResizeSplit {
            tab,
            path: Vec::new(),
            bounds: Rect {
                x: 0.0,
                y: 0.0,
                width: 1.0,
                height: 1.0,
            },
            first_extent: 0.5,
        },
        Command::ResetSplit {
            tab,
            path: Vec::new(),
        },
        Command::SetPaneState {
            pane,
            state: PaneState::default(),
        },
    ]
}

#[test]
fn only_arrangement_changes_are_structural() {
    for command in tab_commands().into_iter().chain(pane_commands()) {
        assert!(command.is_structural(), "{command:?}");
    }
    for command in view_only() {
        assert!(!command.is_structural(), "{command:?}");
    }
}
