use crate::input::geometry::ViewPoint;
use crate::input::modifiers::Modifiers;
use crate::input::pointer::mouse_action::MouseAction;
use crate::input::pointer::mouse_button::MouseButton;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct MouseEvent {
    pub button: MouseButton,
    pub action: MouseAction,
    pub point: ViewPoint,
    pub modifiers: Modifiers,
}
