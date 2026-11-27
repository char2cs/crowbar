use gpui::{Pixels, Point};
use layout::{Command, TabId};

use crate::gesture::TabDrop;

#[derive(Debug, Clone, PartialEq)]
#[non_exhaustive]
pub enum Event {
    Apply(Vec<Command>),
    ToggleSidebar,
    DragOut { tab: TabId, position: Point<Pixels> },
    DropOut { tab: TabId, position: Point<Pixels> },
    DragCancel,
    DropChat { chat: String, at: TabDrop },
}
