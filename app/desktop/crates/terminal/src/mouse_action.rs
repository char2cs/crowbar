//! What the mouse did.

/// The kind of mouse event.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum MouseAction {
    /// A button went down (or the wheel turned).
    Press,
    /// A button came up.
    Release,
    /// The pointer moved with a button held.
    Drag,
    /// The pointer moved with no button held.
    Move,
}
