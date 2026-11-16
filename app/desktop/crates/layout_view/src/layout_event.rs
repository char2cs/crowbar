use model::{Command, Placement, TabId};

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
}
