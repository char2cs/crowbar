use std::rc::Rc;

use gpui::{App, Hsla, Window};
use ui::IconName;

pub(crate) type HandleAction = Rc<dyn Fn(&mut Window, &mut App)>;

pub(crate) struct HandleButton {
    pub(crate) kind: &'static str,
    pub(crate) tip: &'static str,
    pub(crate) glyph: (IconName, f32),
    pub(crate) ink: Hsla,
    pub(crate) action: HandleAction,
}
