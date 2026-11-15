//! A mouse event over the grid.

use crate::modifiers::Modifiers;
use crate::mouse_action::MouseAction;
use crate::mouse_button::MouseButton;
use crate::view_point::ViewPoint;

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
