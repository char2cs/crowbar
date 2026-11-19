//! The blink phase a field keeps for its caret.

use crate::control::editor::field::Blink;

pub(crate) trait Blinking: Sized + 'static {
    fn blink(&self) -> &Blink;
    fn blink_mut(&mut self) -> &mut Blink;
}
