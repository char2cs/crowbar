//! Whether a space's host answers.

/// Unreachable spaces grey out their rows but still accept clicks.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum Reach {
    /// The host answers.
    #[default]
    Reachable,
    /// The host does not answer.
    Unreachable,
}
