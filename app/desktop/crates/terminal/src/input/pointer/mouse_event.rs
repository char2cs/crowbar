//! A mouse event over the grid.

use crate::input::geometry::ViewPoint;
use crate::input::modifiers::Modifiers;
use crate::input::pointer::mouse_action::MouseAction;
use crate::input::pointer::mouse_button::MouseButton;

/// A mouse event at a cell, ready to be reported to a program that tracks the mouse.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct MouseEvent {
    /// The button involved.
    pub button: MouseButton,
    /// What happened to it.
    pub action: MouseAction,
    /// The cell under the pointer.
    pub point: ViewPoint,
    /// Modifiers held.
    pub modifiers: Modifiers,
}
