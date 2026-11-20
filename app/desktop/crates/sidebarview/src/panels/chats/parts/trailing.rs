//! Whether a row keeps its hover buttons while an inline input replaces its label.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Trailing {
    /// A rename keeps them.
    Kept,
    /// A branch-name or draft input hides them.
    Hidden,
}
