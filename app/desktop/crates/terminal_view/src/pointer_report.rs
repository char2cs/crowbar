//! A mouse report on its way to the program.

use terminal::{MouseButton, ViewPoint};

#[derive(Debug, Clone, Copy)]
pub(crate) struct PointerReport {
    pub(crate) button: MouseButton,
    pub(crate) point: ViewPoint,
    pub(crate) modifiers: terminal::Modifiers,
}
