//! The two screen buffers of a terminal.

/// Which buffer is showing.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ScreenBuffer {
    /// The main screen, with scrollback.
    Primary,
    /// The full-screen application buffer, with no scrollback.
    Alternate,
}
