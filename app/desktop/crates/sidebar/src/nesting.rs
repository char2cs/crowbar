//! Whether a row has children to fold.

/// Parents carry a fold chevron.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum Nesting {
    /// No children.
    #[default]
    Leaf,
    /// Has children.
    Parent,
}
