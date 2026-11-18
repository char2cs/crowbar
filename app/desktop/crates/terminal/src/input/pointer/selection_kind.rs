//! How a selection grows.

/// The unit a selection snaps to.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SelectionKind {
    /// Character by character.
    Simple,
    /// Whole words.
    Word,
    /// Whole lines.
    Line,
}
