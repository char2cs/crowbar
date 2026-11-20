use gpui::{Hsla, SharedString};
use layout::DragSubject;

use crate::tabbar::tab_actions::TabDrop;

pub(crate) struct TabDragging {
    pub(crate) subject: DragSubject,
    pub(crate) ghost: SharedString,
    pub(crate) line: Hsla,
    pub(crate) drop: TabDrop,
}
