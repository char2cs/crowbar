use layout::{Command, Placement, TabId};
use protocol::PaneId;

use ideshell::PathAction;

/// What a [`LayoutView`](crate::LayoutView) asks its owner to do.
#[derive(Debug, Clone, PartialEq)]
#[non_exhaustive]
pub enum LayoutEvent {
    /// Apply this command to the layout, then give the view the new state.
    Apply(Command),
    /// A chat row of the sidebar was dropped on a pane: open the chat in `tab` at `placement`,
    /// or move it there when it is already open.
    OpenChat {
        /// The id of the chat.
        chat: String,
        /// The tab that receives the chat.
        tab: TabId,
        /// The pane and side the chat lands next to.
        placement: Placement,
    },
    /// A file row of the Files panel was dropped on a pane: open the file as a tab of the group
    /// holding `onto`, or show it where the view already shows it.
    OpenFile {
        /// The file, relative to the Files panel's workspace.
        path: String,
        /// The pane it was dropped on.
        onto: PaneId,
    },
    /// A chat row of the sidebar was dropped on the empty stage: open the chat as its own tab,
    /// or show it where it is already open.
    StageChat {
        /// The id of the chat.
        chat: String,
    },
    /// A view tab dragged out of its strip was dropped on a pane of the shown tab: move the
    /// chats of `tab` into `onto` at `placement`.
    DropTab {
        /// The dragged view tab.
        tab: TabId,
        /// The shown tab, which receives the chats.
        onto: TabId,
        /// The pane and side the chats land next to.
        placement: Placement,
    },
    /// A strip tab was pressed, or reached with the arrow keys: focus `pane`, and let the strip
    /// keep the keyboard instead of the page that becomes active.
    StripFocus {
        /// The tab's pane.
        pane: PaneId,
    },
    /// A tab menu row asked to do `action` with the file at `path`, relative to its workspace.
    TabPath {
        /// What to do.
        action: PathAction,
        /// The file, relative to its workspace.
        path: String,
    },
    /// A tab menu row asked to rename the tab of `pane`, which is now called `current`.
    RenameTab {
        /// The tab's pane.
        pane: PaneId,
        /// The tab's present name.
        current: String,
    },
    /// A tab menu row asked to read the file of `pane` from disk again.
    ReloadTab {
        /// The tab's pane.
        pane: PaneId,
    },
    /// The pointer moved onto or off a pane, and [`LayoutView::handle_hole`] moved with it.
    ///
    /// [`LayoutView::handle_hole`]: crate::LayoutView::handle_hole
    Hovered,
    /// Something the user should be told, such as why a key did nothing.
    Announce(String),
    /// The "+" button of a strip was pressed: open a New Tab page as a new tab of the group
    /// holding `onto`.
    NewTab {
        /// Any pane of the group whose strip's button was pressed.
        onto: PaneId,
    },
    /// The "Review this branch" button of a strip was pressed: open the branch review of the
    /// group holding `onto`, as a tab of that group.
    ReviewBranch {
        /// Any pane of the group whose strip's button was pressed.
        onto: PaneId,
    },
}
