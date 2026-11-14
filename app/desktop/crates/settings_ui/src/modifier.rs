//! A modifier key held during a key press.

/// One of the modifier keys.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Modifier {
    /// Control.
    Control,
    /// Alt or Option.
    Alt,
    /// Shift.
    Shift,
    /// Command on macOS, the logo key elsewhere.
    Platform,
}
