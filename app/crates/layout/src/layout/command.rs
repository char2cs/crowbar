mod layout_execute;
pub(crate) mod pane_commands;
pub(super) mod tab_commands;

use protocol::{PaneId, PaneState, Params};

use crate::ide::presentation::editor_view::EditorView;
use crate::layout::tab::pane_instance::PaneInstance;
use crate::space_id::SpaceId;
use crate::tab_id::TabId;
use crate::tree::branch::Branch;
use crate::tree::geometry::rect::Rect;
use crate::tree::placement::Placement;

#[derive(Debug, Clone, PartialEq)]
pub enum Command {
    AddTab {
        id: TabId,
        title: String,
        space: SpaceId,
    },
    CloseTab {
        tab: TabId,
    },
    ActivateTab {
        tab: TabId,
    },
    MoveTab {
        tab: TabId,
        to_index: usize,
    },
    RenameTab {
        tab: TabId,
        title: String,
    },
    OpenPane {
        tab: TabId,
        instance: PaneInstance,
        placement: Option<Placement>,
    },
    OpenPaneInGroup {
        instance: PaneInstance,
        onto: PaneId,
        index: Option<usize>,
    },
    MoveToGroup {
        pane: PaneId,
        onto: PaneId,
        index: Option<usize>,
    },
    ClosePane {
        pane: PaneId,
    },
    ClosePanes {
        panes: Vec<PaneId>,
    },
    PinPane {
        pane: PaneId,
    },
    UnpinPane {
        pane: PaneId,
    },
    SetEditorView {
        pane: PaneId,
        view: EditorView,
    },
    ResizeChat {
        pane: PaneId,
        share: f32,
    },
    PreviewPane {
        pane: PaneId,
    },
    KeepPane {
        pane: PaneId,
    },
    FocusPane {
        pane: PaneId,
    },
    MovePane {
        pane: PaneId,
        placement: Placement,
    },
    MovePaneToTab {
        pane: PaneId,
        to: TabId,
        placement: Option<Placement>,
    },
    DetachPane {
        pane: PaneId,
        new_tab: TabId,
        title: String,
    },
    SwapPanes {
        a: PaneId,
        b: PaneId,
    },
    ResizeSplit {
        tab: TabId,
        path: Vec<Branch>,
        bounds: Rect,
        first_extent: f32,
    },
    ResetSplit {
        tab: TabId,
        path: Vec<Branch>,
    },
    AdoptSpace {
        tab: TabId,
        space: SpaceId,
    },
    SetPaneState {
        pane: PaneId,
        state: PaneState,
    },
    SetPaneParams {
        pane: PaneId,
        params: Params,
    },
}

impl Command {
    #[must_use]
    pub fn is_structural(&self) -> bool {
        !matches!(
            self,
            Self::ActivateTab { .. }
                | Self::FocusPane { .. }
                | Self::PreviewPane { .. }
                | Self::KeepPane { .. }
                | Self::ResizeSplit { .. }
                | Self::ResetSplit { .. }
                | Self::ResizeChat { .. }
                | Self::SetPaneState { .. }
                | Self::SetPaneParams { .. }
                | Self::AdoptSpace { .. }
        )
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::tree::edge::Edge;

    fn instance(pane: &PaneId) -> PaneInstance {
        PaneInstance {
            pane_id: pane.clone(),
            type_id: protocol::PaneTypeId::new("core.chat").unwrap(),
            params: Params::default(),
            state: PaneState::default(),
        }
    }

    fn tab_commands() -> [Command; 4] {
        let tab = TabId(1);
        [
            Command::AddTab {
                id: tab,
                title: String::new(),
                space: SpaceId::default(),
            },
            Command::CloseTab { tab },
            Command::MoveTab { tab, to_index: 0 },
            Command::RenameTab {
                tab,
                title: String::new(),
            },
        ]
    }

    fn pane_commands() -> Vec<Command> {
        let pane = PaneId::new("a").unwrap();
        let other = PaneId::new("b").unwrap();
        let tab = TabId(1);
        let placement = Placement {
            target: other.clone(),
            edge: Edge::Left,
        };
        let mut commands = group_commands(&pane, &other);
        commands.extend([
            Command::OpenPane {
                tab,
                instance: instance(&pane),
                placement: None,
            },
            Command::ClosePane { pane: pane.clone() },
            Command::ClosePanes {
                panes: vec![pane.clone()],
            },
            Command::PinPane { pane: pane.clone() },
            Command::UnpinPane { pane: pane.clone() },
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
            Command::SwapPanes { a: pane, b: other },
        ]);
        commands
    }

    fn group_commands(
        pane: &PaneId,
        other: &PaneId,
    ) -> Vec<Command> {
        vec![
            Command::OpenPaneInGroup {
                instance: instance(pane),
                onto: other.clone(),
                index: None,
            },
            Command::MoveToGroup {
                pane: pane.clone(),
                onto: other.clone(),
                index: None,
            },
            Command::SetEditorView {
                pane: pane.clone(),
                view: EditorView::Open,
            },
        ]
    }

    fn view_only() -> [Command; 6] {
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
                pane: pane.clone(),
                state: PaneState::default(),
            },
            Command::SetPaneParams {
                pane,
                params: Params::default(),
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
}
