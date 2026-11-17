use std::rc::Rc;

use gpui::{App, Window};
use model::DragSubject;

pub(crate) type TabAction = Rc<dyn Fn(&mut Window, &mut App)>;
pub(crate) type StripKeys = Rc<dyn Fn(&str, &mut Window, &mut App)>;
pub(crate) type TabDrop = Rc<dyn Fn(&DragSubject, &mut Window, &mut App)>;

pub(crate) struct TabActions {
    pub(crate) select: TabAction,
    pub(crate) close: TabAction,
    pub(crate) remove: TabAction,
    pub(crate) drop: TabDrop,
}
