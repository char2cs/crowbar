use protocol::PaneId;

use crate::layout::tab::pane_instance::PaneInstance;
use crate::tab_id::TabId;

#[derive(Debug, Clone, PartialEq)]
pub enum DragSubject {
    Pane(PaneId),
    Open(PaneInstance),
    Chat(String),
    Tab(TabId),
    File(String),
}
