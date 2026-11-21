use std::rc::Rc;

use gpui::{App, Window};
use protocol::PaneId;

/// What a press on a part of the tab bar does.
pub type TabAction = Rc<dyn Fn(&mut Window, &mut App)>;
/// What a key pressed while the tab bar holds the focus does, given the key's name.
pub type StripKeys = Rc<dyn Fn(&str, &mut Window, &mut App)>;
/// What releasing a tab dragged along the bar does: it moves the tab to the index it landed on.
pub type TabMove = Rc<dyn Fn(&PaneId, usize, &mut Window, &mut App)>;

/// What one editor tab does.
pub struct TabActions {
    /// A press on the tab shows it.
    pub select: TabAction,
    /// The end button closes or unpins it.
    pub close: TabAction,
    /// A middle press closes it.
    pub remove: TabAction,
    /// A double click keeps it as an ordinary tab; only a preview tab has one.
    pub keep: Option<TabAction>,
}

ui::opaque_debug!(TabActions);
