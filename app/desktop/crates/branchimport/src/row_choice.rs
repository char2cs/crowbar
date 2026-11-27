use gpui::{App, Window};
use ui::Pose;

pub(crate) type Toggle = Box<dyn Fn(&mut Window, &mut App)>;

pub(crate) struct RowChoice {
    pub(crate) picked: Pose,
    pub(crate) locked: Pose,
    pub(crate) toggle: Toggle,
    pub(crate) toggle_lock: Toggle,
}
