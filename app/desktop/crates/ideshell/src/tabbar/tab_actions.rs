use std::rc::Rc;

use gpui::{App, Window};
use protocol::PaneId;

pub type TabAction = Rc<dyn Fn(&mut Window, &mut App)>;
pub type StripKeys = Rc<dyn Fn(&str, &mut Window, &mut App)>;
pub type TabMove = Rc<dyn Fn(&PaneId, usize, &mut Window, &mut App)>;

pub struct TabActions {
    pub select: TabAction,
    pub close: TabAction,
    pub remove: TabAction,
    pub keep: Option<TabAction>,
}

ui::opaque_debug!(TabActions);
