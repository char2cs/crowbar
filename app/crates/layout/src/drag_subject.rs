use protocol::PaneId;

use crate::layout::tab::pane_instance::PaneInstance;
use crate::tab_id::TabId;

/// What a pointer drag carries. The sidebar, the tab bar and the pane layout all start and
/// accept drags of this one shape, so a drop resolves to a [`Command`](crate::Command) the same
/// way wherever it lands.
#[derive(Debug, Clone, PartialEq)]
pub enum DragSubject {
    /// A pane that is already open, dragged by its handle.
    Pane(PaneId),
    /// A pane that is not open yet, for instance a chat dragged out of the sidebar.
    Open(PaneInstance),
    /// A chat dragged out of the sidebar by its id; the owner of the layout builds its pane.
    Chat(String),
    /// A whole tab, dragged inside the tab bar.
    Tab(TabId),
    /// A file of the workspace, by its relative path, dragged out of the Files panel; it opens
    /// in the pane it is dropped on, never in a pane of its own.
    File(String),
}
