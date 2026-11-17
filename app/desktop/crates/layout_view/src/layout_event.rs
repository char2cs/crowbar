use model::{Command, Placement, TabId};
use protocol::PaneId;

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
    /// A strip tab was pressed, or reached with the arrow keys: focus `pane`, and let the strip
    /// keep the keyboard instead of the page that becomes active.
    StripFocus {
        /// The tab's pane.
        pane: PaneId,
    },
    /// Something the user should be told, such as why a key did nothing.
    Announce(String),
    /// The "+" button of a strip was pressed: open a New Tab page as a new tab of the group
    /// holding `onto`.
    NewTab {
        /// Any pane of the group whose strip's button was pressed.
        onto: PaneId,
    },
}
