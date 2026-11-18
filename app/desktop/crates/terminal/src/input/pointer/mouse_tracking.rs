//! Which mouse events the running program asked for.

/// The mouse-tracking mode the program set (DECSET 1000, 1002, 1003).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum MouseTracking {
    /// The program does not track the mouse.
    Off,
    /// Presses and releases.
    Click,
    /// Presses, releases and motion while a button is held.
    Drag,
    /// All motion.
    Motion,
}
