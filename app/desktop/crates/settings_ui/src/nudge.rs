//! Direction of one stepper press.

/// Which way a number stepper moves.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Nudge {
    /// Add one step.
    Up,
    /// Subtract one step.
    Down,
}
