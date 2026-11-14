use protocol::{PaneId, PaneState};

use crate::branch::Branch;
use crate::pane_instance::PaneInstance;
use crate::placement::Placement;
use crate::tab_id::TabId;

/// Everything that can change a [`Layout`](crate::Layout). Commands are applied atomically.
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
    /// Adds a pane to a tab and focuses it.
    OpenPane {
        /// The tab that receives the pane.
        tab: TabId,
        /// The pane to add.
        instance: PaneInstance,
        /// Where to put it; `None` splits the focused pane to the right.
        placement: Option<Placement>,
    },
    /// Removes a pane.
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
    /// Moves a pane into another tab, keeping its id and state.
    MovePaneToTab {
        /// The pane to move.
        pane: PaneId,
        /// The destination tab.
        to: TabId,
        /// Where it lands; `None` splits the destination's focused pane to the right.
        placement: Option<Placement>,
    },
    /// Exchanges two panes of one tab.
    SwapPanes {
        /// First pane.
        a: PaneId,
        /// Second pane.
        b: PaneId,
    },
    /// Sets the share of one split.
    ResizeSplit {
        /// The tab that owns the split.
        tab: TabId,
        /// Steps from the root to the split.
        path: Vec<Branch>,
        /// Share of the first child.
        ratio: f32,
    },
    /// Replaces a pane's persisted state.
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
                | Self::SetPaneState { .. }
        )
    }
}

#[cfg(test)]
#[path = "command_tests.rs"]
mod tests;
