use std::rc::Rc;

use gpui::{App, Window};
use layout::DragSubject;

/// What a press on a part of the tab bar does.
pub type TabAction = Rc<dyn Fn(&mut Window, &mut App)>;
/// What a key pressed while the tab bar holds the focus does, given the key's name.
pub type StripKeys = Rc<dyn Fn(&str, &mut Window, &mut App)>;
/// What dropping a dragged subject on a tab, or on the bar's free end, does.
pub type TabDrop = Rc<dyn Fn(&DragSubject, &mut Window, &mut App)>;

/// What one editor tab does.
pub struct TabActions {
    /// A press on the tab shows it.
    pub select: TabAction,
    /// The end button closes or unpins it.
    pub close: TabAction,
    /// A middle press closes it.
    pub remove: TabAction,
    /// A drop on it moves the dragged tab there.
    pub drop: TabDrop,
}

ui::opaque_debug!(TabActions);
