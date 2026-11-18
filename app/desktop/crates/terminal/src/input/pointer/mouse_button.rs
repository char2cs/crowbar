//! Mouse buttons a terminal program can see.

/// A button, or the absence of one for plain motion.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum MouseButton {
    /// Primary button.
    Left,
    /// Wheel click.
    Middle,
    /// Secondary button.
    Right,
    /// Wheel rolled up.
    WheelUp,
    /// Wheel rolled down.
    WheelDown,
    /// No button is held (motion only).
    None,
}
