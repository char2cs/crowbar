use protocol::{PaneId, PaneState};

use crate::branch::Branch;
use crate::pane_instance::PaneInstance;
use crate::placement::Placement;
use crate::rect::Rect;
use crate::tab_id::TabId;

/// Everything that can change a [`Layout`](crate::Layout). A command is applied completely or
/// not at all, and one command is one undo step.
///
/// A command that takes the last pane out of a tab closes that tab too: the layout never
/// holds an empty tab unless [`Command::AddTab`] just made it.
#[derive(Debug, Clone, PartialEq)]
pub enum Command {
    /// Adds an empty tab at the end and makes it active.
    AddTab {
        /// The new tab's id.
        id: TabId,
        /// Its title.
        title: String,
    },
    /// Removes a tab with its panes; the neighbouring tab becomes active.
    CloseTab {
        /// The tab to remove.
        tab: TabId,
    },
    /// Makes a tab active.
    ActivateTab {
        /// The tab to show.
        tab: TabId,
    },
    /// Moves a tab to a new position in the tab bar.
    MoveTab {
        /// The tab to move.
        tab: TabId,
        /// Its new index.
        to_index: usize,
    },
    /// Renames a tab.
    RenameTab {
        /// The tab to rename.
        tab: TabId,
        /// The new title.
        title: String,
    },
    /// Shows a pane in a tab and focuses it. When a pane with the same type and parameters is
    /// already open (parameters are what identifies a chat, a file, a thread), that pane is
    /// moved to the placement instead and `instance` is dropped, so one thing is never open
    /// twice. Dropping a pane next to itself only focuses it.
    OpenPane {
        /// The tab that receives the pane.
        tab: TabId,
        /// The pane to add.
        instance: PaneInstance,
        /// Where to put it; `None` splits the focused pane to the right.
        placement: Option<Placement>,
    },
    /// Removes a pane, and its tab when it was the tab's last pane. Focus goes to the
    /// neighbour that takes the freed space; when the tab closes, the adjacent tab is shown.
    ClosePane {
        /// The pane to remove.
        pane: PaneId,
    },
    /// Focuses a pane and activates its tab.
    FocusPane {
        /// The pane to focus.
        pane: PaneId,
    },
    /// Moves a pane next to another pane of the same tab.
    MovePane {
        /// The pane to move.
        pane: PaneId,
        /// Where it lands.
        placement: Placement,
    },
    /// Moves a pane into another tab, keeping its id and state, and shows that tab. The source
    /// tab closes when the pane was its last.
    MovePaneToTab {
        /// The pane to move.
        pane: PaneId,
        /// The destination tab.
        to: TabId,
        /// Where it lands; `None` splits the destination's focused pane to the right.
        placement: Option<Placement>,
    },
    /// Moves a pane out of its siblings into a new tab right of its current one, and shows
    /// it. Refused for the only pane of a tab.
    DetachPane {
        /// The pane to detach.
        pane: PaneId,
        /// The id of the tab that is created.
        new_tab: TabId,
        /// The new tab's title.
        title: String,
    },
    /// Exchanges two panes of one tab.
    SwapPanes {
        /// First pane.
        a: PaneId,
        /// Second pane.
        b: PaneId,
    },
    /// Gives the first child of a split a size in pixels, held to the minimum pane size. Send
    /// it for every step of a drag and for its release: the committed ratio is the last one
    /// shown.
    ResizeSplit {
        /// The tab that owns the split.
        tab: TabId,
        /// Steps from the root to the split.
        path: Vec<Branch>,
        /// The rectangle the tab is currently laid out in.
        bounds: Rect,
        /// Wanted size of the split's first child along its axis.
        first_extent: f32,
    },
    /// Gives a split's two children equal shares again.
    ResetSplit {
        /// The tab that owns the split.
        tab: TabId,
        /// Steps from the root to the split.
        path: Vec<Branch>,
    },
    /// Replaces a pane's persisted state. Never an undo step, however often it is sent.
    SetPaneState {
        /// The pane to update.
        pane: PaneId,
        /// The new state.
        state: PaneState,
    },
}

impl Command {
    /// Whether the command changes the arrangement, as opposed to focus, sizes or pane state.
    /// Only structural commands can be undone.
    #[must_use]
    pub fn is_structural(&self) -> bool {
        !matches!(
            self,
            Self::ActivateTab { .. }
                | Self::FocusPane { .. }
                | Self::ResizeSplit { .. }
                | Self::ResetSplit { .. }
                | Self::SetPaneState { .. }
        )
    }
}

#[cfg(test)]
mod tests {
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
}
