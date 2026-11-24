use protocol::{PaneId, PaneState, Params};

use crate::branch::Branch;
use crate::ide::editor_view::EditorView;
use crate::pane_instance::PaneInstance;
use crate::placement::Placement;
use crate::rect::Rect;
use crate::space_id::SpaceId;
use crate::tab_id::TabId;

/// Everything that can change a [`Layout`](crate::Layout). A command is applied completely or
/// not at all, and one command is one undo step.
///
/// A command that takes the last pane out of a tab closes that tab too: the layout never
/// holds an empty tab unless [`Command::AddTab`] just made it.
#[derive(Debug, Clone, PartialEq)]
pub enum Command {
    /// Adds an empty tab at the end and shows it in its space.
    AddTab {
        /// The new tab's id.
        id: TabId,
        /// Its title.
        title: String,
        /// The space it belongs to, for good.
        space: SpaceId,
    },
    /// Removes a tab with its panes; when its space showed it, the neighbouring tab of that space
    /// is shown, or the space's stage when it has no other tab.
    CloseTab {
        /// The tab to remove.
        tab: TabId,
    },
    /// Shows a tab in its space.
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
    /// Opens a pane as a tab of the strip of the group that holds `onto`, at `index` among that
    /// group's panes (the end when `None`), shows it and focuses it. A pane with the same type
    /// and parameters that is already open anywhere is moved there instead and `instance` is
    /// dropped.
    OpenPaneInGroup {
        /// The pane to add.
        instance: PaneInstance,
        /// Any pane of the group that receives it.
        onto: PaneId,
        /// Its position in the strip.
        index: Option<usize>,
    },
    /// Moves an open pane into the group that holds `onto`, in this tab or another, at `index`
    /// among the other panes of that group (the end when `None`). Inside one group it only
    /// reorders the strip. The pane is shown and focused, its tab is shown in its space, and a source
    /// tab left without panes closes.
    MoveToGroup {
        /// The pane to move.
        pane: PaneId,
        /// Any pane of the destination group.
        onto: PaneId,
        /// Its position in the strip.
        index: Option<usize>,
    },
    /// Removes a pane, and its tab when it was the tab's last pane. Focus goes to the
    /// neighbour that takes the freed space; when the tab closes, the adjacent tab is shown.
    ClosePane {
        /// The pane to remove.
        pane: PaneId,
    },
    /// Removes several panes at once, as one undo step: a bulk close of a strip, or of a whole
    /// group. Every pane must exist and appear once, or nothing happens. A tab left without
    /// panes closes; focus goes where closing the panes one by one would put it.
    ClosePanes {
        /// The panes to remove.
        panes: Vec<PaneId>,
    },
    /// Pins a pane of a strip: it moves behind the already pinned tabs and stays in front of
    /// the others. Does nothing when it is pinned.
    PinPane {
        /// The pane to pin.
        pane: PaneId,
    },
    /// Unpins a pane of a strip: it becomes the first of the loose tabs. Does nothing when it
    /// is not pinned.
    UnpinPane {
        /// The pane to unpin.
        pane: PaneId,
    },
    /// Opens or closes the editor view of the group holding `pane`, as the chat header's open
    /// editor button and the split toggle do. An open editor with no tab is empty.
    SetEditorView {
        /// A member of the group.
        pane: PaneId,
        /// Whether the editor view shows.
        view: EditorView,
    },
    /// Makes an editor tab its group's preview, as develop's file tree opens a file with a
    /// single click; the group's previous preview becomes an ordinary tab.
    PreviewPane {
        /// The editor tab.
        pane: PaneId,
    },
    /// Keeps a pane as an ordinary tab, as develop does on a double-click or a first edit: no
    /// editor tab anywhere stays a preview.
    KeepPane {
        /// The pane that was kept.
        pane: PaneId,
    },
    /// Focuses a pane, shows it in its group and shows its tab in that tab's space.
    FocusPane {
        /// The pane to focus.
        pane: PaneId,
    },
    /// Moves a pane next to another pane of the same tab, out of its group when it shares one.
    MovePane {
        /// The pane to move.
        pane: PaneId,
        /// Where it lands.
        placement: Placement,
    },
    /// Moves a pane into another tab, keeping its id and state, and shows that tab. A chat takes
    /// its editor tabs with it. The source tab closes when the pane was its last.
    MovePaneToTab {
        /// The pane to move.
        pane: PaneId,
        /// The destination tab.
        to: TabId,
        /// Where it lands; `None` splits the destination's focused pane to the right.
        placement: Option<Placement>,
    },
    /// Moves a chat and its editor tabs out of its siblings into a new tab of the same space,
    /// right of its current one, and shows it. Refused for an editor tab and for the only chat of a tab.
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
    /// Gives a tab stored before spaces were recorded the space it belongs to, once; when its
    /// empty space showed it, `space` shows it unless it already shows a tab. Never an undo
    /// step, and the undo history learns the tab's space too.
    AdoptSpace {
        /// A tab with no space yet.
        tab: TabId,
        /// The space it belongs to.
        space: SpaceId,
    },
    /// Replaces a pane's persisted state. Never an undo step, however often it is sent.
    SetPaneState {
        /// The pane to update.
        pane: PaneId,
        /// The new state.
        state: PaneState,
    },
    /// Re-keys a pane on new parameters, as a draft chat does when its chat is created. The pane
    /// keeps its id, place and state. Never an undo step.
    SetPaneParams {
        /// The pane to re-key.
        pane: PaneId,
        /// Its parameters from now on.
        params: Params,
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
                | Self::PreviewPane { .. }
                | Self::KeepPane { .. }
                | Self::ResizeSplit { .. }
                | Self::ResetSplit { .. }
                | Self::SetPaneState { .. }
                | Self::SetPaneParams { .. }
                | Self::AdoptSpace { .. }
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
